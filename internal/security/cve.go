package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"easyserver/internal/auth"
	"easyserver/internal/firewall"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"
)

// Service provides security-audit operations (CVE scanning, kernel status,
// login anomaly detection).
type Service struct {
	exec     executor.CommandExecutor
	firewall *firewall.Service
	auth     *auth.AuthService
}

// NewService creates a security Service.
func NewService(exec executor.CommandExecutor, firewallSvc *firewall.Service, authSvc *auth.AuthService) *Service {
	return &Service{exec: exec, firewall: firewallSvc, auth: authSvc}
}

// Vulnerability is one installed package with known CVEs.
type Vulnerability struct {
	Package string   `json:"package"`
	Version string   `json:"version"`
	VulnIDs []string `json:"vuln_ids"`
}

type installedPackage struct {
	Name    string
	Version string
}

// Scan lists installed apt packages and queries osv.dev for known CVEs.
// Returns only packages that have at least one vulnerability.
func (s *Service) Scan(ctx context.Context) ([]Vulnerability, error) {
	pkgs, err := s.listInstalled(ctx)
	if err != nil {
		return nil, apperror.ErrInternal.WithMessage("读取已装包失败: " + err.Error())
	}
	var vulns []Vulnerability
	// osv.dev querybatch accepts up to 1000 queries per request.
	for i := 0; i < len(pkgs); i += 500 {
		end := i + 500
		if end > len(pkgs) {
			end = len(pkgs)
		}
		results, err := s.queryOSVBatch(ctx, pkgs[i:end])
		if err != nil {
			return nil, apperror.ErrInternal.WithMessage("查询 osv.dev 失败: " + err.Error())
		}
		for j, res := range results {
			if len(res.Vulns) == 0 {
				continue
			}
			ids := make([]string, 0, len(res.Vulns))
			for _, v := range res.Vulns {
				ids = append(ids, v.ID)
			}
			vulns = append(vulns, Vulnerability{
				Package: pkgs[i+j].Name,
				Version: pkgs[i+j].Version,
				VulnIDs: ids,
			})
		}
	}
	return vulns, nil
}

// listInstalled parses `apt list --installed` output.
func (s *Service) listInstalled(ctx context.Context) ([]installedPackage, error) {
	out, _, err := s.exec.RunCombined(ctx, "apt", "list", "--installed")
	if err != nil {
		return nil, err
	}
	var pkgs []installedPackage
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		// format: name/version arch [status]
		parts := strings.SplitN(line, "/", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		fields := strings.Fields(parts[1])
		if len(fields) < 1 {
			continue
		}
		pkgs = append(pkgs, installedPackage{Name: name, Version: fields[0]})
	}
	return pkgs, nil
}

type osvQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

type osvVuln struct {
	ID string `json:"id"`
}

type osvResult struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvBatchResp struct {
	Results []osvResult `json:"results"`
}

// queryOSVBatch queries osv.dev for a batch of packages (Debian ecosystem).
func (s *Service) queryOSVBatch(ctx context.Context, pkgs []installedPackage) ([]osvResult, error) {
	queries := make([]osvQuery, len(pkgs))
	for i, p := range pkgs {
		queries[i].Package.Name = p.Name
		queries[i].Package.Ecosystem = "Debian"
		queries[i].Version = p.Version
	}
	body, _ := json.Marshal(map[string]any{"queries": queries})
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.osv.dev/v1/querybatch", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r osvBatchResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Results, nil
}

// Upgrade runs apt-get install --only-upgrade for the given packages.
func (s *Service) Upgrade(ctx context.Context, packages []string) (string, error) {
	if len(packages) == 0 {
		return "", apperror.ErrBadRequest.WithMessage("未指定升级的包")
	}
	args := append([]string{"install", "--only-upgrade", "-y"}, packages...)
	out, _, err := s.exec.RunCombined(ctx, "apt-get", args...)
	if err != nil {
		return out, apperror.ErrInternal.WithMessage("升级失败: " + err.Error())
	}
	return out, nil
}

// KernelStatus reports the running kernel vs the latest installed kernel.
type KernelStatus struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsReboot bool   `json:"needs_reboot"`
}

// KernelStatus returns running kernel and latest installed kernel.
func (s *Service) KernelStatus(ctx context.Context) (*KernelStatus, error) {
	out, _, _ := s.exec.RunCombined(ctx, "uname", "-r")
	current := strings.TrimSpace(out)
	out, _, _ = s.exec.RunCombined(ctx, "bash", "-c", "dpkg -l 'linux-image-*' 2>/dev/null | awk '/^ii/{print $2}' | sed 's/linux-image-//' | sort -V | tail -1")
	latest := strings.TrimSpace(out)
	return &KernelStatus{
		Current:     current,
		Latest:      latest,
		NeedsReboot: current != "" && latest != "" && current != latest,
	}, nil
}

// PackageUpdateCount returns pending apt upgrades (apt list --upgradable).
func (s *Service) PackageUpdateCount(ctx context.Context) (int, error) {
	out, _, err := s.exec.RunCombined(ctx, "apt", "list", "--upgradable")
	if err != nil {
		return 0, fmt.Errorf("apt list --upgradable: %w", err)
	}
	count := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		count++
	}
	return count, nil
}
