package firewall

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"easyserver/internal/infra/errx"
)

// ============================================================================
// 端口转发（nftables nat DNAT）
//
// 规则存 SQLite（权威），内核侧使用独立表 `inet easyserver_nat`，与
// Docker / 系统的 nat 表完全隔离。每条内核规则带 `easyserver-pf[-fwd|-masq]-<id>`
// 注释标签，删除/禁用时按标签定位 handle 精确清理。
//
// 三条内核规则：
//   prerouting  : [iifname X] [ip saddr C] <proto> dport P dnat to DIP:DPORT
//   filter FWD  : ip daddr DIP <proto> dport DPORT accept   （FORWARD 策略收紧后仍可用）
//   postrouting : ip daddr DIP <proto> dport DPORT masquerade（仅目标是内网地址时，
//                 解决内网客户端经公网 IP 访问的 hairpin 回程）
// ============================================================================

const (
	pfNatFamily     = "inet"
	pfNatTable      = "easyserver_nat"
	pfPrerouting    = "prerouting"
	pfPostrouting   = "postrouting"
	pfCommentPrefix = "easyserver-pf"
)

// PortForwardRule is one DNAT port-forwarding rule.
type PortForwardRule struct {
	ID         int64  `json:"id"`
	Protocol   string `json:"protocol"`    // tcp, udp
	ListenPort string `json:"listen_port"` // 单端口或范围（a-b）
	DestIP     string `json:"dest_ip"`
	DestPort   string `json:"dest_port"` // 空 = 同监听端口
	SourceCIDR string `json:"source_cidr"`
	Interface  string `json:"interface"`
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	// 列表展示用：来自 nft 计数器
	Packets int64 `json:"packets"`
	Bytes   int64 `json:"bytes"`
}

// pfCounter 是一条转发规则的 nft 命中计数。
type pfCounter struct {
	Packets int64
	Bytes   int64
}

// PortForwardRepository is the data access interface for port-forward rules.
type PortForwardRepository interface {
	List(ctx context.Context) ([]PortForwardRule, error)
	Get(ctx context.Context, id int64) (*PortForwardRule, error)
	Create(ctx context.Context, rule *PortForwardRule) error
	Update(ctx context.Context, rule *PortForwardRule) error
	Delete(ctx context.Context, id int64) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	ListEnabled(ctx context.Context) ([]PortForwardRule, error)
}

// PortForwardService manages nftables DNAT port forwarding.
type PortForwardService struct {
	repo          PortForwardRepository
	firewall      *Service // 复用受保护端口判定
	backendWarned atomic.Bool
}

// NewPortForwardService creates a PortForwardService.
func NewPortForwardService(repo PortForwardRepository, firewallSvc *Service) *PortForwardService {
	return &PortForwardService{repo: repo, firewall: firewallSvc}
}

// ----------------------------------------------------------------------------
// 校验
// ----------------------------------------------------------------------------

var (
	pfPortRe  = regexp.MustCompile(`^\d{1,5}$`)
	pfRangeRe = regexp.MustCompile(`^(\d{1,5})-(\d{1,5})$`)
	pfIfaceRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,15}$`)
)

// pfPortSpan 解析端口表达式（单端口或范围），返回起止端口。
func pfPortSpan(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if m := pfPortRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(s)
		if n < 1 || n > 65535 {
			return 0, 0, errx.BadRequest("端口 %s 超出范围（1-65535）", s)
		}
		return n, n, nil
	}
	if m := pfRangeRe.FindStringSubmatch(s); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a >= b || a < 1 || b > 65535 {
			return 0, 0, errx.BadRequest("端口范围 %s 无效（需 a-b 且 a < b）", s)
		}
		return a, b, nil
	}
	return 0, 0, errx.BadRequest("端口格式无效: %s", s)
}

// validate 检查规则字段；返回监听起始端口。
func (s *PortForwardService) validate(ctx context.Context, rule *PortForwardRule) (int, error) {
	rule.Protocol = strings.ToLower(strings.TrimSpace(rule.Protocol))
	if rule.Protocol != "tcp" && rule.Protocol != "udp" {
		return 0, errx.BadRequest("协议仅支持 tcp / udp")
	}
	listenStart, _, err := pfPortSpan(rule.ListenPort)
	if err != nil {
		return 0, err
	}

	rule.DestIP = strings.TrimSpace(rule.DestIP)
	ip := net.ParseIP(rule.DestIP)
	if ip == nil || ip.To4() == nil {
		return 0, errx.BadRequest("目标地址必须是有效的 IPv4 地址")
	}

	// 目标端口：空 = 同监听；范围监听时目标必须为同跨度范围或留空
	rule.DestPort = strings.TrimSpace(rule.DestPort)
	if rule.DestPort != "" {
		ls, le, _ := pfPortSpan(rule.ListenPort)
		ds, de, err := pfPortSpan(rule.DestPort)
		if err != nil {
			return 0, err
		}
		if le-ls != de-ds {
			return 0, errx.BadRequest("目标端口范围的跨度必须与监听端口一致")
		}
	}

	rule.SourceCIDR = strings.TrimSpace(rule.SourceCIDR)
	if rule.SourceCIDR != "" {
		if _, _, err := net.ParseCIDR(rule.SourceCIDR); err != nil && net.ParseIP(rule.SourceCIDR) == nil {
			return 0, errx.BadRequest("来源限制必须是有效的 CIDR 或 IP 地址")
		}
	}

	rule.Interface = strings.TrimSpace(rule.Interface)
	if rule.Interface != "" && !pfIfaceRe.MatchString(rule.Interface) {
		return 0, errx.BadRequest("网卡名称无效（1-15 位字母/数字/点/横线/下划线）")
	}

	rule.Remark = strings.TrimSpace(rule.Remark)

	// 面板自身受保护端口不允许被监听占用
	if s.firewall != nil && s.firewall.IsProtectedPort(ctx, strconv.Itoa(listenStart)) {
		return 0, errx.BadRequest("监听端口 %d 是面板保护端口，禁止使用", listenStart)
	}
	return listenStart, nil
}

// pfRangesOverlap 判断两个端口区间是否重叠。
func pfRangesOverlap(a1, a2, b1, b2 int) bool {
	return a1 <= b2 && b1 <= a2
}

// checkConflict 与已启用的转发规则比较：协议相同 + 监听区间重叠 + 网卡兼容
// （任一为空视为任意网卡）即视为冲突。
func (s *PortForwardService) checkConflict(ctx context.Context, rule *PortForwardRule, listenStart int) error {
	existing, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	ls, le, _ := pfPortSpan(rule.ListenPort)
	for _, e := range existing {
		if e.ID == rule.ID || e.Protocol != rule.Protocol {
			continue
		}
		es, ee, err := pfPortSpan(e.ListenPort)
		if err != nil {
			continue
		}
		if pfRangesOverlap(ls, le, es, ee) &&
			(rule.Interface == "" || e.Interface == "" || rule.Interface == e.Interface) {
			return errx.Conflict("监听端口 %s/%s 与已有转发规则 #%d 冲突", rule.ListenPort, rule.Protocol, e.ID)
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Service 方法
// ----------------------------------------------------------------------------

// ListPortForwards 返回全部转发规则，附带 nft 计数器。
func (s *PortForwardService) ListPortForwards(ctx context.Context) ([]PortForwardRule, error) {
	rules, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	counters := s.nftCounters(ctx)
	for i := range rules {
		if c, ok := counters[rules[i].ID]; ok {
			rules[i].Packets = c.Packets
			rules[i].Bytes = c.Bytes
		}
	}
	return rules, nil
}

// CreatePortForward 校验、落库并应用内核规则；应用失败则回滚删除。
func (s *PortForwardService) CreatePortForward(ctx context.Context, rule *PortForwardRule) error {
	listenStart, err := s.validate(ctx, rule)
	if err != nil {
		return err
	}
	if err := s.checkConflict(ctx, rule, listenStart); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, rule); err != nil {
		return err
	}
	if err := s.applyForward(ctx, rule); err != nil {
		_ = s.repo.Delete(ctx, rule.ID) // 应用失败回滚，不留死规则
		return err
	}
	return nil
}

// UpdatePortForward 更新规则：先摘除旧内核规则，落库后按新状态重新应用。
func (s *PortForwardService) UpdatePortForward(ctx context.Context, rule *PortForwardRule) error {
	old, err := s.repo.Get(ctx, rule.ID)
	if err != nil {
		return err
	}
	if old == nil {
		return errx.NotFound("转发规则不存在")
	}
	listenStart, err := s.validate(ctx, rule)
	if err != nil {
		return err
	}
	if err := s.checkConflict(ctx, rule, listenStart); err != nil {
		return err
	}
	s.removeForward(ctx, rule.ID)
	if err := s.repo.Update(ctx, rule); err != nil {
		return err
	}
	if rule.Enabled {
		if err := s.applyForward(ctx, rule); err != nil {
			return err
		}
	}
	return nil
}

// DeletePortForward 删除规则并清理内核规则。
func (s *PortForwardService) DeletePortForward(ctx context.Context, id int64) error {
	s.removeForward(ctx, id)
	return s.repo.Delete(ctx, id)
}

// SetPortForwardEnabled 启用/禁用规则并同步内核。
func (s *PortForwardService) SetPortForwardEnabled(ctx context.Context, id int64, enabled bool) error {
	rule, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if rule == nil {
		return errx.NotFound("转发规则不存在")
	}
	if enabled {
		listenStart, err := s.validate(ctx, rule)
		if err != nil {
			return err
		}
		if err := s.checkConflict(ctx, rule, listenStart); err != nil {
			return err
		}
		if err := s.applyForward(ctx, rule); err != nil {
			return err
		}
	} else {
		s.removeForward(ctx, id)
	}
	return s.repo.SetEnabled(ctx, id, enabled)
}

// ReapplyAll 面板启动时重放：清理全部内核侧规则后按库内状态重建，
// 保证内核态与数据库最终一致。任一规则失败不影响其余。
func (s *PortForwardService) ReapplyAll(ctx context.Context) error {
	rules, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, r := range rules {
		s.removeForward(ctx, r.ID)
		if !r.Enabled {
			continue
		}
		if err := s.applyForward(ctx, &r); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ----------------------------------------------------------------------------
// nftables 操作
// ----------------------------------------------------------------------------

// ensureNatTable 确保 nat 表与 prerouting/postrouting 链存在。
func (s *PortForwardService) ensureNatTable(ctx context.Context) error {
	if _, err := execNft(ctx, "list", "table", pfNatFamily, pfNatTable); err == nil {
		return nil
	}
	cmds := [][]string{
		{"add", "table", pfNatFamily, pfNatTable},
		{"add", "chain", pfNatFamily, pfNatTable, pfPrerouting, "{", "type", "nat", "hook", "prerouting", "priority", "dstnat", ";", "}"},
		{"add", "chain", pfNatFamily, pfNatTable, pfPostrouting, "{", "type", "nat", "hook", "postrouting", "priority", "srcnat", ";", "}"},
	}
	for _, args := range cmds {
		if out, err := execNft(ctx, args...); err != nil {
			return fmt.Errorf("nft setup error: %s", out)
		}
	}
	return nil
}

// ensureFilterForward 确保 inet filter 的 FORWARD 链存在（转发放行规则的落点）。
func (s *PortForwardService) ensureFilterForward(ctx context.Context) error {
	if _, err := execNft(ctx, "list", "chain", "inet", "filter", "FORWARD"); err == nil {
		return nil
	}
	for _, args := range [][]string{
		{"add", "table", "inet", "filter"},
		{"add", "chain", "inet", "filter", "FORWARD", "{", "type", "filter", "hook", "forward", "priority", "0", ";", "}"},
	} {
		if out, err := execNft(ctx, args...); err != nil {
			return fmt.Errorf("nft setup error: %s", out)
		}
	}
	return nil
}

// dnatTarget 返回 dnat to 的目标表达式。
func dnatTarget(rule *PortForwardRule) string {
	if rule.DestPort != "" && rule.DestPort != rule.ListenPort {
		return rule.DestIP + ":" + rule.DestPort
	}
	return rule.DestIP
}

// applyForward 在内核中创建该规则的三条 nft 规则。
func (s *PortForwardService) applyForward(ctx context.Context, rule *PortForwardRule) error {
	if err := s.ensureNatTable(ctx); err != nil {
		return err
	}
	if err := s.ensureFilterForward(ctx); err != nil {
		return err
	}

	proto := rule.Protocol
	comment := "\"" + pfCommentPrefix + "-" + strconv.FormatInt(rule.ID, 10) + "\""

	// 1) prerouting DNAT
	args := []string{"add", "rule", pfNatFamily, pfNatTable, pfPrerouting}
	if rule.Interface != "" {
		args = append(args, "iifname", rule.Interface)
	}
	if rule.SourceCIDR != "" {
		args = append(args, "ip", "saddr", rule.SourceCIDR)
	}
	// inet 族表中 dnat 的族关键词位于 dnat 与 to 之间（dnat ip to A.B.C.D:P）
	args = append(args, proto, "dport", rule.ListenPort, "dnat", "ip", "to", dnatTarget(rule), "comment", comment)
	if out, err := execNft(ctx, args...); err != nil {
		return fmt.Errorf("nft dnat error: %s", out)
	}

	// 2) filter FORWARD 放行（FORWARD 策略收紧后仍可用）
	fwdComment := "\"" + pfCommentPrefix + "-fwd-" + strconv.FormatInt(rule.ID, 10) + "\""
	args = []string{"add", "rule", "inet", "filter", "FORWARD", "ip", "daddr", rule.DestIP, proto, "dport", rule.DestPortOrListen(), "accept", "comment", fwdComment}
	if out, err := execNft(ctx, args...); err != nil {
		return fmt.Errorf("nft forward accept error: %s", out)
	}

	// 3) postrouting masquerade（仅目标是内网地址：内网客户端经公网 IP 访问的回程）
	if destIsPrivate(rule.DestIP) {
		masqComment := "\"" + pfCommentPrefix + "-masq-" + strconv.FormatInt(rule.ID, 10) + "\""
		args = []string{"add", "rule", pfNatFamily, pfNatTable, pfPostrouting, "ip", "daddr", rule.DestIP, proto, "dport", rule.DestPortOrListen(), "masquerade", "comment", masqComment}
		if out, err := execNft(ctx, args...); err != nil {
			return fmt.Errorf("nft masquerade error: %s", out)
		}
	}
	return nil
}

// DestPortOrListen 返回目标端口（空则取监听端口）。
func (r *PortForwardRule) DestPortOrListen() string {
	if r.DestPort != "" {
		return r.DestPort
	}
	return r.ListenPort
}

// destIsPrivate 报告 IPv4 地址是否属于 RFC1918 私有网段。
func destIsPrivate(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	return ip != nil && ip.IsPrivate()
}

// removeForward 按注释标签定位并删除该规则的全部内核规则。
// 标签形如 easyserver-pf-<id>"（nft 注释字面量含引号），避免 -1 误伤 -12。
func (s *PortForwardService) removeForward(ctx context.Context, id int64) {
	tag := pfCommentPrefix + "-" + strconv.FormatInt(id, 10) + "\""
	type chainRef struct{ family, table, chain string }
	chains := []chainRef{
		{pfNatFamily, pfNatTable, pfPrerouting},
		{pfNatFamily, pfNatTable, pfPostrouting},
		{"inet", "filter", "FORWARD"},
	}
	handleRe := regexp.MustCompile(`# handle (\d+)`)
	for _, c := range chains {
		out, err := execNft(ctx, "-a", "list", "chain", c.family, c.table, c.chain)
		if err != nil {
			continue // 链/表不存在即无需清理
		}
		for line := range strings.SplitSeq(out, "\n") {
			if !strings.Contains(line, tag) {
				continue
			}
			if m := handleRe.FindStringSubmatch(line); m != nil {
				// nft delete rule 必须带 handle 关键字
				_, _ = execNft(ctx, "delete", "rule", c.family, c.table, c.chain, "handle", m[1])
			}
		}
	}
}

// nftCounters 从 prerouting 规则中提取每条转发的命中计数。
func (s *PortForwardService) nftCounters(ctx context.Context) map[int64]pfCounter {
	counters := map[int64]pfCounter{}
	out, err := execNft(ctx, "-a", "list", "chain", pfNatFamily, pfNatTable, pfPrerouting)
	if err != nil {
		return counters
	}
	idRe := regexp.MustCompile(pfCommentPrefix + `-(\d+)"`)
	cntRe := regexp.MustCompile(`counter packets (\d+) bytes (\d+)`)
	for line := range strings.SplitSeq(out, "\n") {
		id := idRe.FindStringSubmatch(line)
		cnt := cntRe.FindStringSubmatch(line)
		if id == nil || cnt == nil {
			continue
		}
		n, _ := strconv.ParseInt(id[1], 10, 64)
		p, _ := strconv.ParseInt(cnt[1], 10, 64)
		b, _ := strconv.ParseInt(cnt[2], 10, 64)
		counters[n] = pfCounter{Packets: p, Bytes: b}
	}
	return counters
}

// execNft 执行 nft 命令。表/链名均为代码内常量或已校验值，无注入面。
func execNft(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "nft", args...).CombinedOutput()
	return string(out), err
}

// WarnBackendOnce 若系统以 ufw 为防火墙管理工具，提示端口转发仍由 nftables 承载。
func (s *PortForwardService) WarnBackendOnce(ctx context.Context) {
	if s.backendWarned.Swap(true) {
		return
	}
	if s.firewall != nil && s.firewall.DetectTool(ctx) == "ufw" {
		log.Printf("firewall: 检测到 ufw —— 端口转发规则由 nftables 直接承载，ufw 侧不管理这些规则")
	}
}
