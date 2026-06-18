package service

import (
	"fmt"
	"log"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
)

type CloudService struct {
	client     *lighthouse.Client
	instanceID string
	region     string
}

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

type FirewallRule struct {
	RuleID   string `json:"rule_id"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Source   string `json:"source"`
	Action   string `json:"action"`
	Remark   string `json:"remark"`
}

type SnapshotInfo struct {
	SnapshotID string `json:"snapshot_id"`
	Name       string `json:"name"`
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
	DiskGB     int    `json:"disk_gb"`
	CreatedAt  string `json:"created_at"`
}

type TrafficInfo struct {
	PackageTotalGB     int    `json:"package_total_gb"`
	PackageUsedGB      int    `json:"package_used_gb"`
	PackageRemainingGB int    `json:"package_remaining_gb"`
	PackageExpiredAt   string `json:"package_expired_at"`
}

type MonitorData struct {
	Metric    string         `json:"metric"`
	Unit      string         `json:"unit"`
	Points    []MonitorPoint `json:"points"`
}

type MonitorPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

func NewCloudService(secretID, secretKey, region, instanceID string) (*CloudService, error) {
	credential := common.NewCredential(secretID, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "lighthouse.tencentcloudapi.com"

	client, err := lighthouse.NewClient(credential, region, cpf)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud client: %w", err)
	}

	return &CloudService{
		client:     client,
		instanceID: instanceID,
		region:     region,
	}, nil
}

// GetInstances returns all instances
func (s *CloudService) GetInstances() ([]InstanceInfo, error) {
	request := lighthouse.NewDescribeInstancesRequest()
	request.Limit = common.Int64Ptr(100)

	response, err := s.client.DescribeInstances(request)
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

// GetInstance returns a specific instance
func (s *CloudService) GetInstance(instanceID string) (*InstanceInfo, error) {
	request := lighthouse.NewDescribeInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	response, err := s.client.DescribeInstances(request)
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

// StartInstance starts an instance
func (s *CloudService) StartInstance(instanceID string) error {
	request := lighthouse.NewStartInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.StartInstances(request)
	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	log.Printf("cloud: started instance %s", instanceID)
	return nil
}

// StopInstance stops an instance
func (s *CloudService) StopInstance(instanceID string) error {
	request := lighthouse.NewStopInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.StopInstances(request)
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	log.Printf("cloud: stopped instance %s", instanceID)
	return nil
}

// RestartInstance restarts an instance
func (s *CloudService) RestartInstance(instanceID string) error {
	request := lighthouse.NewRebootInstancesRequest()
	request.InstanceIds = common.StringPtrs([]string{instanceID})

	_, err := s.client.RebootInstances(request)
	if err != nil {
		return fmt.Errorf("failed to restart instance: %w", err)
	}

	log.Printf("cloud: restarted instance %s", instanceID)
	return nil
}

// GetFirewallRules returns firewall rules for an instance
func (s *CloudService) GetFirewallRules(instanceID string) ([]FirewallRule, error) {
	request := lighthouse.NewDescribeFirewallRulesRequest()
	request.InstanceId = common.StringPtr(instanceID)

	response, err := s.client.DescribeFirewallRules(request)
	if err != nil {
		return nil, fmt.Errorf("failed to describe firewall rules: %w", err)
	}

	var rules []FirewallRule
	for _, rule := range response.Response.FirewallRuleSet {
		r := FirewallRule{
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

// AddFirewallRule adds a firewall rule
func (s *CloudService) AddFirewallRule(instanceID string, rule FirewallRule) error {
	request := lighthouse.NewCreateFirewallRulesRequest()
	request.InstanceId = common.StringPtr(instanceID)

	firewallRule := &lighthouse.FirewallRule{
		Protocol: common.StringPtr(rule.Protocol),
		Port:     common.StringPtr(rule.Port),
		CidrBlock: common.StringPtr(rule.Source),
		Action:   common.StringPtr(rule.Action),
	}

	if rule.Remark != "" {
		firewallRule.FirewallRuleDescription = common.StringPtr(rule.Remark)
	}

	request.FirewallRules = []*lighthouse.FirewallRule{firewallRule}

	_, err := s.client.CreateFirewallRules(request)
	if err != nil {
		return fmt.Errorf("failed to create firewall rule: %w", err)
	}

	log.Printf("cloud: added firewall rule for instance %s", instanceID)
	return nil
}

// DeleteFirewallRule deletes a firewall rule
func (s *CloudService) DeleteFirewallRule(instanceID string, ruleID string) error {
	// Note: Tencent Cloud API doesn't support direct rule ID deletion
	// Need to get current rules, remove the target, and update
	// This is a simplified implementation
	return fmt.Errorf("firewall rule deletion not implemented")
}

// GetSnapshots returns snapshots for an instance
func (s *CloudService) GetSnapshots(instanceID string) ([]SnapshotInfo, error) {
	// Note: Tencent Cloud Lighthouse API for snapshots may differ
	// This is a placeholder implementation
	return []SnapshotInfo{}, nil
}

// CreateSnapshot creates a snapshot
func (s *CloudService) CreateSnapshot(instanceID, name string) error {
	// Note: Tencent Cloud Lighthouse API for snapshot creation may differ
	// This is a placeholder implementation
	return fmt.Errorf("snapshot creation not implemented yet")
}

// ApplySnapshot applies a snapshot (rollback)
func (s *CloudService) ApplySnapshot(snapshotID string) error {
	// Note: Tencent Cloud Lighthouse API for snapshot rollback may differ
	// This is a placeholder implementation
	return fmt.Errorf("snapshot rollback not implemented yet")
}

// GetMonitorData returns monitor data for an instance
func (s *CloudService) GetMonitorData(instanceID, metric string, start, end time.Time) (*MonitorData, error) {
	// Note: Tencent Cloud Lighthouse API for monitor data may differ
	// This is a placeholder implementation
	return &MonitorData{
		Metric: metric,
		Unit:   "%",
		Points: []MonitorPoint{},
	}, nil
}

// GetTraffic returns traffic package info
func (s *CloudService) GetTraffic(instanceID string) (*TrafficInfo, error) {
	// Note: Tencent Cloud Lighthouse API for traffic may differ
	// This is a placeholder implementation
	return &TrafficInfo{}, nil
}
