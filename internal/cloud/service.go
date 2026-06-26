package cloud

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
	tcmonitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

// InstanceInfo represents a cloud instance.
type InstanceInfo struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Region     string `json:"region"`
	PublicIP   string `json:"public_ip"`
	PrivateIP  string `json:"private_ip"`
	CPU        int    `json:"cpu"`
	MemoryGB   int    `json:"memory_gb"`
	DiskGB     int    `json:"disk_gb"`
	CreatedAt  string `json:"created_at"`
	ExpiredAt  string `json:"expired_at"`
}

// FirewallRule represents a cloud firewall rule.
type FirewallRule struct {
	RuleID   string `json:"rule_id"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Source   string `json:"source"`
	Action   string `json:"action"`
	Remark   string `json:"remark"`
}

// SnapshotInfo represents a cloud snapshot.
type SnapshotInfo struct {
	SnapshotID string `json:"snapshot_id"`
	Name       string `json:"name"`
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
	DiskGB     int    `json:"disk_gb"`
	CreatedAt  string `json:"created_at"`
}

// TrafficInfo represents traffic package information.
type TrafficInfo struct {
	PackageTotalGB     int    `json:"package_total_gb"`
	PackageUsedGB      int    `json:"package_used_gb"`
	PackageRemainingGB int    `json:"package_remaining_gb"`
	PackageExpiredAt   string `json:"package_expired_at"`
}

// MonitorData represents monitor data.
type MonitorData struct {
	Metric string         `json:"metric"`
	Unit   string         `json:"unit"`
	Points []MonitorPoint `json:"points"`
}

// MonitorPoint represents a single monitor data point.
type MonitorPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// Service manages Tencent Cloud Lighthouse operations.
type Service struct {
	client     *lighthouse.Client
	credential *common.Credential
	instanceID string
	region     string
}

// NewService creates a new cloud Service.
func NewService(secretID, secretKey, region, instanceID string) (*Service, error) {
	credential := common.NewCredential(secretID, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "lighthouse.tencentcloudapi.com"

	client, err := lighthouse.NewClient(credential, region, cpf)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud client: %w", err)
	}

	return &Service{
		client:     client,
		credential: credential,
		instanceID: instanceID,
		region:     region,
	}, nil
}

// GetInstances returns all instances.
func (s *Service) GetInstances(ctx context.Context) ([]InstanceInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewDescribeInstancesRequest()
	request.Limit = common.Int64Ptr(100)

	response, err := s.client.DescribeInstancesWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	var instances []InstanceInfo
	for _, inst := range response.Response.InstanceSet {
		instance := InstanceInfo{
			InstanceID: *inst.InstanceId,
			Name:       *inst.InstanceName,
			State:      *inst.InstanceState,
			Region:     s.region,
			CPU:        int(*inst.CPU),
			MemoryGB:   int(*inst.Memory),
			DiskGB:     int(*inst.SystemDisk.DiskSize),
			CreatedAt:  *inst.CreatedTime,
			ExpiredAt:  *inst.ExpiredTime,
		}

		if len(inst.PublicAddresses) > 0 {
			instance.PublicIP = *inst.PublicAddresses[0]
		}
		if len(inst.PrivateAddresses) > 0 {
			instance.PrivateIP = *inst.PrivateAddresses[0]
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// GetInstance returns a specific instance.
func (s *Service) GetInstance(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewDescribeInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	response, err := s.client.DescribeInstancesWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(response.Response.InstanceSet) == 0 {
		return nil, fmt.Errorf("instance not found")
	}

	inst := response.Response.InstanceSet[0]
	instance := &InstanceInfo{
		InstanceID: *inst.InstanceId,
		Name:       *inst.InstanceName,
		State:      *inst.InstanceState,
		Region:     s.region,
		CPU:        int(*inst.CPU),
		MemoryGB:   int(*inst.Memory),
		DiskGB:     int(*inst.SystemDisk.DiskSize),
		CreatedAt:  *inst.CreatedTime,
		ExpiredAt:  *inst.ExpiredTime,
	}

	if len(inst.PublicAddresses) > 0 {
		instance.PublicIP = *inst.PublicAddresses[0]
	}
	if len(inst.PrivateAddresses) > 0 {
		instance.PrivateIP = *inst.PrivateAddresses[0]
	}

	return instance, nil
}

// StartInstance starts an instance.
func (s *Service) StartInstance(ctx context.Context, instanceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewStartInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.StartInstancesWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	log.Printf("cloud: started instance %s", instanceID)
	return nil
}

// StopInstance stops an instance.
func (s *Service) StopInstance(ctx context.Context, instanceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewStopInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.StopInstancesWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	log.Printf("cloud: stopped instance %s", instanceID)
	return nil
}

// RestartInstance restarts an instance.
func (s *Service) RestartInstance(ctx context.Context, instanceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewRebootInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.RebootInstancesWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to restart instance: %w", err)
	}

	log.Printf("cloud: restarted instance %s", instanceID)
	return nil
}

// GetFirewallRules returns firewall rules for an instance.
func (s *Service) GetFirewallRules(ctx context.Context, instanceID string) ([]FirewallRule, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewDescribeFirewallRulesRequest()
	request.InstanceId = common.StringPtr(instanceID)

	response, err := s.client.DescribeFirewallRulesWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe firewall rules: %w", err)
	}

	var rules []FirewallRule
	for i, rule := range response.Response.FirewallRuleSet {
		r := FirewallRule{
			RuleID:   strconv.Itoa(i),
			Protocol: *rule.Protocol,
			Port:     *rule.Port,
			Source:   *rule.CidrBlock,
			Action:   *rule.Action,
		}

		if rule.FirewallRuleDescription != nil {
			r.Remark = *rule.FirewallRuleDescription
		}

		rules = append(rules, r)
	}

	return rules, nil
}

// AddFirewallRule adds a firewall rule.
func (s *Service) AddFirewallRule(ctx context.Context, instanceID string, rule FirewallRule) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewCreateFirewallRulesRequest()
	request.InstanceId = common.StringPtr(instanceID)

	firewallRule := &lighthouse.FirewallRule{
		Protocol:  common.StringPtr(rule.Protocol),
		Port:      common.StringPtr(rule.Port),
		CidrBlock: common.StringPtr(rule.Source),
		Action:    common.StringPtr(rule.Action),
	}

	if rule.Remark != "" {
		firewallRule.FirewallRuleDescription = common.StringPtr(rule.Remark)
	}

	request.FirewallRules = []*lighthouse.FirewallRule{firewallRule}

	_, err := s.client.CreateFirewallRulesWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to create firewall rule: %w", err)
	}

	log.Printf("cloud: added firewall rule for instance %s", instanceID)
	return nil
}

// DeleteFirewallRule deletes a firewall rule by its index-based ID.
func (s *Service) DeleteFirewallRule(ctx context.Context, instanceID string, ruleID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	describeReq := lighthouse.NewDescribeFirewallRulesRequest()
	describeReq.InstanceId = common.StringPtr(instanceID)
	describeReq.Limit = common.Int64Ptr(100)

	describeResp, err := s.client.DescribeFirewallRulesWithContext(ctx, describeReq)
	if err != nil {
		return fmt.Errorf("failed to list firewall rules for deletion: %w", err)
	}

	rules := describeResp.Response.FirewallRuleSet
	if len(rules) == 0 {
		return fmt.Errorf("no firewall rules found for instance %s", instanceID)
	}

	idx, err := strconv.Atoi(ruleID)
	if err != nil || idx < 0 || idx >= len(rules) {
		return fmt.Errorf("invalid firewall rule ID %q: must be 0-%d", ruleID, len(rules)-1)
	}

	targetRule := rules[idx]

	deleteRule := &lighthouse.FirewallRule{
		Protocol:  targetRule.Protocol,
		Port:      targetRule.Port,
		CidrBlock: targetRule.CidrBlock,
		Action:    targetRule.Action,
	}

	deleteReq := lighthouse.NewDeleteFirewallRulesRequest()
	deleteReq.InstanceId = common.StringPtr(instanceID)
	deleteReq.FirewallRules = []*lighthouse.FirewallRule{deleteRule}

	_, err = s.client.DeleteFirewallRulesWithContext(ctx, deleteReq)
	if err != nil {
		return fmt.Errorf("failed to delete firewall rule #%s: %w", ruleID, err)
	}

	log.Printf("cloud: deleted firewall rule #%s for instance %s (protocol=%s port=%s source=%s action=%s)",
		ruleID, instanceID,
		derefString(targetRule.Protocol), derefString(targetRule.Port),
		derefString(targetRule.CidrBlock), derefString(targetRule.Action))
	return nil
}

// GetSnapshots returns snapshots for an instance.
func (s *Service) GetSnapshots(ctx context.Context, instanceID string) ([]SnapshotInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewDescribeSnapshotsRequest()
	request.Filters = []*lighthouse.Filter{
		{
			Name:   common.StringPtr("instance-id"),
			Values: common.StringPtrs([]string{instanceID}),
		},
	}

	response, err := s.client.DescribeSnapshotsWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe snapshots: %w", err)
	}

	var snapshots []SnapshotInfo
	for _, snap := range response.Response.SnapshotSet {
		info := SnapshotInfo{
			SnapshotID: derefString(snap.SnapshotId),
			InstanceID: instanceID,
			Status:     derefString(snap.SnapshotState),
			DiskGB:     derefInt64(snap.DiskSize),
		}
		if snap.SnapshotName != nil {
			info.Name = *snap.SnapshotName
		}
		snapshots = append(snapshots, info)
	}

	return snapshots, nil
}

// CreateSnapshot creates a snapshot for an instance.
func (s *Service) CreateSnapshot(ctx context.Context, instanceID, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewCreateInstanceSnapshotRequest()
	request.InstanceId = common.StringPtr(instanceID)
	request.SnapshotName = common.StringPtr(name)

	_, err := s.client.CreateInstanceSnapshotWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	log.Printf("cloud: created snapshot %q for instance %s", name, instanceID)
	return nil
}

// ApplySnapshot applies a snapshot (rollback an instance to a snapshot).
func (s *Service) ApplySnapshot(ctx context.Context, snapshotID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	describeReq := lighthouse.NewDescribeSnapshotsRequest()
	describeReq.SnapshotIds = common.StringPtrs([]string{snapshotID})

	describeResp, err := s.client.DescribeSnapshotsWithContext(ctx, describeReq)
	if err != nil {
		return fmt.Errorf("failed to describe snapshot %s: %w", snapshotID, err)
	}

	if len(describeResp.Response.SnapshotSet) == 0 {
		return fmt.Errorf("snapshot %s not found", snapshotID)
	}

	request := lighthouse.NewApplyInstanceSnapshotRequest()
	request.InstanceId = common.StringPtr(s.instanceID)
	request.SnapshotId = common.StringPtr(snapshotID)

	_, err = s.client.ApplyInstanceSnapshotWithContext(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to apply snapshot: %w", err)
	}

	log.Printf("cloud: applied snapshot %s to instance %s", snapshotID, s.instanceID)
	return nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt64(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

var metricConfig = map[string]struct {
	TCMetric string
	Unit     string
}{
	"CPU_USAGE":      {TCMetric: "CpuUsage", Unit: "%"},
	"MEMORY_USAGE":   {TCMetric: "MemUsage", Unit: "%"},
	"DISK_USAGE":     {TCMetric: "DiskUsage", Unit: "%"},
	"NETWORK_IN_OUT": {TCMetric: "LanOuttraffic", Unit: "Bps"},
}

// GetMonitorData returns monitor data for an instance.
func (s *Service) GetMonitorData(ctx context.Context, instanceID, metric string, start, end time.Time) (*MonitorData, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	mc, ok := metricConfig[metric]
	if !ok {
		return nil, fmt.Errorf("unsupported metric: %s (supported: CPU_USAGE, MEMORY_USAGE, DISK_USAGE, NETWORK_IN_OUT)", metric)
	}

	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "monitor.tencentcloudapi.com"
	monitorClient, err := tcmonitor.NewClient(s.credential, s.region, cpf)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitor client: %w", err)
	}

	request := tcmonitor.NewGetMonitorDataRequest()
	request.Namespace = common.StringPtr("QCE/LIGHTHOUSE")
	request.MetricName = common.StringPtr(mc.TCMetric)
	request.Period = common.Uint64Ptr(60)
	request.StartTime = common.StringPtr(start.Format(time.RFC3339))
	request.EndTime = common.StringPtr(end.Format(time.RFC3339))
	request.Instances = []*tcmonitor.Instance{
		{
			Dimensions: []*tcmonitor.Dimension{
				{
					Name:  common.StringPtr("InstanceId"),
					Value: common.StringPtr(instanceID),
				},
			},
		},
	}

	response, err := monitorClient.GetMonitorDataWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitor data for metric %s: %w", metric, err)
	}

	data := &MonitorData{
		Metric: metric,
		Unit:   mc.Unit,
		Points: make([]MonitorPoint, 0),
	}

	if response.Response != nil {
		for _, dp := range response.Response.DataPoints {
			if dp == nil {
				continue
			}
			for i, ts := range dp.Timestamps {
				if ts == nil {
					continue
				}
				value := 0.0
				if i < len(dp.Values) && dp.Values[i] != nil {
					value = *dp.Values[i]
				}
				data.Points = append(data.Points, MonitorPoint{
					Timestamp: time.Unix(int64(*ts), 0).Format(time.RFC3339),
					Value:     value,
				})
			}
		}
	}

	log.Printf("cloud: retrieved %d monitor data points for instance %s (metric=%s)", len(data.Points), instanceID, metric)
	return data, nil
}

// GetTraffic returns traffic package info for an instance.
func (s *Service) GetTraffic(ctx context.Context, instanceID string) (*TrafficInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := lighthouse.NewDescribeInstancesTrafficPackagesRequest()
	if instanceID != "" {
		request.InstanceIds = common.StringPtrs([]string{instanceID})
	}

	response, err := s.client.DescribeInstancesTrafficPackagesWithContext(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe traffic packages: %w", err)
	}

	if len(response.Response.InstanceTrafficPackageSet) == 0 {
		log.Printf("cloud: no traffic packages found for instance %s", instanceID)
		return &TrafficInfo{}, nil
	}

	instanceTraffic := response.Response.InstanceTrafficPackageSet[0]

	const bytesPerGB int64 = 1024 * 1024 * 1024
	var totalBytes, usedBytes, remainingBytes int64
	var latestExpiry string

	for _, pkg := range instanceTraffic.TrafficPackageSet {
		if pkg.TrafficPackageTotal != nil {
			totalBytes += *pkg.TrafficPackageTotal
		}
		if pkg.TrafficUsed != nil {
			usedBytes += *pkg.TrafficUsed
		}
		if pkg.TrafficPackageRemaining != nil {
			remainingBytes += *pkg.TrafficPackageRemaining
		}
		if pkg.EndTime != nil && *pkg.EndTime > latestExpiry {
			latestExpiry = *pkg.EndTime
		}
	}

	info := &TrafficInfo{
		PackageTotalGB:     int(totalBytes / bytesPerGB),
		PackageUsedGB:      int(usedBytes / bytesPerGB),
		PackageRemainingGB: int(remainingBytes / bytesPerGB),
		PackageExpiredAt:   latestExpiry,
	}

	log.Printf("cloud: retrieved traffic info for instance %s (total=%dGB, used=%dGB, remaining=%dGB)",
		instanceID, info.PackageTotalGB, info.PackageUsedGB, info.PackageRemainingGB)
	return info, nil
}
