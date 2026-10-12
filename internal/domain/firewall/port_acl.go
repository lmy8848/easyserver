package firewall

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// PortACL 是"暴露端口访问控制"条目：面板管控插件暴露/服务的端口，
// 仅列出的授权源 IP/CIDR 可访问该端口，其余来源一律 DROP。
// 由插件经 Host API port.expose / port.revoke 维护（权限 firewall.manage），
// 也供未来任何需要受控暴露端口的插件复用。
type PortACL struct {
	ID        int64
	Port      int
	Protocol  string // tcp | udp
	CIDRs     []string
	Remark    string
	CreatedAt string
}

// PortACLRepository 持久化暴露端口白名单。
type PortACLRepository interface {
	Upsert(ctx context.Context, acl *PortACL) error
	Get(ctx context.Context, port int, protocol string) (*PortACL, error)
	Delete(ctx context.Context, port int, protocol string) error
	List(ctx context.Context) ([]PortACL, error)
}

const aclCommentPrefix = "easyserver-acl"

// PortACLService 应用暴露端口访问控制（nftables inet filter INPUT 链）。
type PortACLService struct {
	repo PortACLRepository
}

func NewPortACLService(repo PortACLRepository) *PortACLService {
	return &PortACLService{repo: repo}
}

// Expose 登记并应用端口白名单（先清旧规则再应用，幂等）。
func (s *PortACLService) Expose(ctx context.Context, port int, protocol string, cidrs []string, remark string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "tcp" && protocol != "udp" {
		return fmt.Errorf("协议 %q 无效（仅 tcp/udp）", protocol)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口 %d 无效（需 1-65535）", port)
	}
	clean, err := normalizeCIDRs(cidrs)
	if err != nil {
		return err
	}
	acl := &PortACL{Port: port, Protocol: protocol, CIDRs: clean, Remark: remark}
	if err := s.repo.Upsert(ctx, acl); err != nil {
		return err
	}
	return s.apply(ctx, acl)
}

// Revoke 移除端口白名单（DB + 内核规则）。
func (s *PortACLService) Revoke(ctx context.Context, port int, protocol string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	existing, err := s.repo.Get(ctx, port, protocol)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	s.remove(ctx, existing.ID)
	return s.repo.Delete(ctx, port, protocol)
}

// List 返回全部白名单条目。
func (s *PortACLService) List(ctx context.Context) ([]PortACL, error) {
	return s.repo.List(ctx)
}

// ReapplyAll 启动时重放：先清内核全部 acl 规则，再按库内条目重新应用。
func (s *PortACLService) ReapplyAll(ctx context.Context) error {
	all, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, a := range all {
		s.remove(ctx, a.ID)
	}
	for i := range all {
		if err := s.apply(ctx, &all[i]); err != nil {
			return err
		}
	}
	return nil
}

// apply 为端口落地 nftables 规则：授权源 ACCEPT（多个）+ 其余来源 DROP 兜底。
// 规则按"先 ACCEPT 后 DROP"追加，保证同端口内顺序正确（DROP 在最后兜底）。
func (s *PortACLService) apply(ctx context.Context, acl *PortACL) error {
	if err := ensureFilterInput(ctx); err != nil {
		return err
	}
	comment := "\"" + aclCommentPrefix + "-" + strconv.FormatInt(acl.ID, 10) + "\""
	for _, cidr := range acl.CIDRs {
		args := []string{"add", "rule", "inet", "filter", "INPUT",
			acl.Protocol, "dport", strconv.Itoa(acl.Port), "ip", "saddr", cidr, "accept", "comment", comment}
		if out, err := execNft(ctx, args...); err != nil {
			return fmt.Errorf("nft accept error: %s", out)
		}
	}
	denyComment := "\"" + aclCommentPrefix + "-deny-" + strconv.FormatInt(acl.ID, 10) + "\""
	args := []string{"add", "rule", "inet", "filter", "INPUT",
		acl.Protocol, "dport", strconv.Itoa(acl.Port), "drop", "comment", denyComment}
	if out, err := execNft(ctx, args...); err != nil {
		return fmt.Errorf("nft drop error: %s", out)
	}
	return nil
}

// remove 按注释标签清理某条目在 INPUT 链上的全部内核规则。
// 清理是 best-effort：链不存在（尚未初始化）即无需清理，不返回错误。
func (s *PortACLService) remove(ctx context.Context, id int64) {
	out, err := execNft(ctx, "-a", "list", "chain", "inet", "filter", "INPUT")
	if err != nil {
		return // 链/表不存在即无需清理
	}
	idStr := strconv.FormatInt(id, 10)
	// 标签形如 easyserver-acl-<id>"（accept）与 easyserver-acl-deny-<id>"（drop）
	tags := []string{
		aclCommentPrefix + "-" + idStr + "\"",
		aclCommentPrefix + "-deny-" + idStr + "\"",
	}
	handleRe := regexp.MustCompile(`# handle (\d+)`)
	for line := range strings.SplitSeq(out, "\n") {
		match := false
		for _, t := range tags {
			if strings.Contains(line, t) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if m := handleRe.FindStringSubmatch(line); m != nil {
			// nft delete rule 必须带 handle 关键字，否则语法错误
			_, _ = execNft(ctx, "delete", "rule", "inet", "filter", "INPUT", "handle", m[1])
		}
	}
}

// normalizeCIDRs 校验并规范化授权源列表：接受单 IP（补 /32）或 CIDR。
func normalizeCIDRs(cidrs []string) ([]string, error) {
	clean := make([]string, 0, len(cidrs))
	seen := map[string]bool{}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			if net.ParseIP(c) == nil {
				return nil, fmt.Errorf("授权源 %q 不是合法 IP/CIDR", c)
			}
			c += "/32"
		}
		seen[c] = true
		clean = append(clean, c)
	}
	if len(clean) == 0 {
		return nil, errors.New("至少需要一个授权源 IP/CIDR")
	}
	return clean, nil
}

// ensureFilterInput 确保 inet filter 表与 INPUT 链存在（nft 规则落地前提）。
func ensureFilterInput(ctx context.Context) error {
	if _, err := execNft(ctx, "list", "table", "inet", "filter"); err == nil {
		return nil
	}
	cmds := [][]string{
		{"add", "table", "inet", "filter"},
		{"add", "chain", "inet", "filter", "INPUT", "{", "type", "filter", "hook", "input", "priority", "0", ";", "}"},
	}
	for _, c := range cmds {
		if _, err := execNft(ctx, c...); err != nil {
			return fmt.Errorf("nft 初始化 INPUT 链失败: %w", err)
		}
	}
	return nil
}
