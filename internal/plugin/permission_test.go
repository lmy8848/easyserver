package plugin

import (
	"context"
	"testing"
)

// 权限审批：HasPermission 校验启用时批准的子集（§11 权限模型），
// 而非 manifest 声明全集。

func TestHasPermissionApprovedSubset(t *testing.T) {
	repo, _ := testRepo(t)
	mgr := NewManager(ManagerOptions{Repo: repo, PluginsDir: t.TempDir()})

	mft := validManifest()
	mft.Permissions = []string{PermAuditWrite, PermExecRestricted, PermConfigReadWrite}
	mgr.manifests[mft.Name] = &mft
	mgr.tokens["token-demo"] = mft.Name

	// 管理员只批准了 audit.write（收缩子集），exec.restricted 与 config 未批准
	if err := repo.UpsertPlugin(context.Background(), &PluginRecord{
		Name:                mft.Name,
		Version:             mft.Version,
		APIVersion:          mft.APIVersion,
		ManifestPath:        "/tmp/demo/plugin.yaml",
		Enabled:             true,
		PermissionsApproved: []string{PermAuditWrite},
	}); err != nil {
		t.Fatal(err)
	}

	if !mgr.HasPermission(context.Background(), "token-demo", PermAuditWrite) {
		t.Error("已批准的 audit.write 应通过")
	}
	if mgr.HasPermission(context.Background(), "token-demo", PermExecRestricted) {
		t.Error("声明但未批准的 exec.restricted 不应通过")
	}
	if mgr.HasPermission(context.Background(), "token-demo", PermConfigReadWrite) {
		t.Error("声明但未批准的 config.readwrite 不应通过")
	}
	if mgr.HasPermission(context.Background(), "token-unknown", PermAuditWrite) {
		t.Error("未知 token 不应通过")
	}
}

func TestApproveRejectsUndefinedPermission(t *testing.T) {
	repo, _ := testRepo(t)
	mgr := NewManager(ManagerOptions{Repo: repo, PluginsDir: t.TempDir()})

	mft := validManifest()
	mft.Permissions = []string{PermAuditWrite}
	mgr.manifests[mft.Name] = &mft

	// 批准未声明的权限应被拒（不可超声明）
	if err := mgr.Approve(context.Background(), mft.Name, []string{PermExecRestricted}); err == nil {
		t.Error("批准未声明权限应被拒绝")
	}
}
