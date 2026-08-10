package database

import (
	"context"
	"fmt"

	"easyserver/internal/infra/task"
)

// 数据库安装的后台执行由通用任务执行器承担（internal/infra/task）：同 DBType
// 去重（key=DBType）、取消、状态机、失败/取消保留至同 key 重装、可选 Task Log
// 附件。这里只保留安装流水线本身 installInstance 与取消入口。

// installInstance runs the container creation pipeline and reports progress
// into log. rt is an install-scoped runtime whose command output is hooked into
// log. The instance row already exists (status "installing", written up front by
// CreateInstance); this flips it to "running" on success / "failed" on error, or
// removes it entirely when the user cancels. ctx is the per-task cancel context
// from the task executor.
func (s *Service) installInstance(ctx context.Context, id int64, dbType DBType, version, image, engineName, containerName, volumeName, password string, spec ContainerSpec, rt DatabaseRuntime, log *task.TaskLog) error {
	canceled := func() bool { return ctx.Err() != nil }
	// removeInstance is the cancel cleanup: drop the container and the instance
	// row — the user aborted, so nothing lingers (a failed install keeps its row
	// for inspection; a canceled one does not).
	removeInstance := func() {
		_ = rt.Remove(context.Background(), engineName, containerName)
		_ = s.repo.DeleteInstance(context.Background(), id)
	}
	fail := func(msg string, err error) error {
		if canceled() {
			removeInstance()
			log.Append("❌ 安装已取消")
			return fmt.Errorf("安装已取消")
		}
		// 失败时保留容器，便于排查失败现场（容器日志还在）。重新安装走
		// "卸载+安装"两步，卸载会先删掉这个残留容器，所以不会被占用卡住。
		_ = s.repo.UpdateInstanceStatus(ctx, id, "failed")
		log.Append("❌ " + msg + ": " + err.Error())
		return err
	}

	log.Append("开始安装 " + image + " ...")
	if err := rt.Create(ctx, spec); err != nil {
		if canceled() {
			removeInstance()
			log.Append("❌ 安装已取消")
			return fmt.Errorf("安装已取消")
		}
		// No container was created — still flip the row to "failed" so the
		// instance doesn't sit at "installing" forever (the log panel surfaces
		// the error and offers reinstall).
		_ = s.repo.UpdateInstanceStatus(ctx, id, "failed")
		log.Append("❌ 创建容器失败: " + err.Error())
		return err
	}
	log.Append("容器已创建，启动服务...")

	if dbType == DBTypeRedis {
		log.Append("写入 Redis 配置...")
		if err := seedRedisConfig(ctx, rt, engineName, containerName, password); err != nil {
			return fail("写入 Redis 配置失败", err)
		}
	}

	if err := rt.Start(ctx, engineName, containerName); err != nil {
		return fail("启动容器失败", err)
	}
	// 等待就绪不设超时：数据库初始化（尤其首次拉镜像后）没有固定时长，卡住时
	// 由容器退出（exited 快失败）或用户取消来终止，而不是倒计时误杀。
	if _, err := waitForHealthy(ctx, rt, engineName, containerName, 0); err != nil {
		return fail("数据库未就绪", err)
	}
	log.Append("✅ 安装完成，数据库已就绪")
	if err := s.repo.UpdateInstanceStatus(ctx, id, "running"); err != nil {
		return err
	}
	return nil
}

// CancelInstall aborts an in-flight install. The goroutine observes the cancel
// at its next command boundary (image pull, create, start, health poll) and
// removes the container and the instance row before finishing — a canceled
// install leaves no row behind, unlike a failed one.
func (s *Service) CancelInstall(installID string) error {
	if !s.taskMgr.Cancel(installID) {
		return fmt.Errorf("安装已结束或不存在")
	}
	return nil
}
