package runtimeenv

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"os/exec"
	"strings"

	"easyserver/internal/infra/task"
)

// buildDepsApt 列出在 Debian/Ubuntu 上从源码编译指定运行时所需的系统包。
// asdf-php 走的是 source build——必须先把工具链装好，否则
// mise install 会停在 buildconf/configure 阶段并抛错。
//
// node / go / java / python 都用预编译二进制，无需本地工具链，因此不在此表。
var buildDepsApt = map[string][]string{
	"php": {
		"build-essential", "autoconf", "bison", "re2c", "pkg-config",
		"libxml2-dev", "libssl-dev", "libicu-dev", "libsqlite3-dev",
		"libcurl4-openssl-dev", "libonig-dev", "libzip-dev", "zlib1g-dev",
		"libgd-dev", "libpq-dev", "libbz2-dev", "libjpeg-dev", "libpng-dev",
		"libreadline-dev", "libtidy-dev", "libxslt1-dev",
	},
}

// hasAptGet 判断是否 Debian/Ubuntu 家族。非该家族（RHEL/Alpine 等）暂不
// 自动装依赖；由后续 mise install 直接尝试，失败后用户从日志看到详细
// 缺包信息再手动准备。
func hasAptGet() bool {
	_, err := os.Stat("/usr/bin/apt-get")
	return err == nil
}

// ensureBuildDeps 在调用 mise install 前确保系统级编译依赖到位。
// 整个过程输出由 runStreaming 写进任务日志（内存流），前端
// SSE 实时可见。
//
// 行为：
//   - lang 不在 buildDepsApt 中（node/go/java）→ 直接返回 nil
//   - 非 apt 系统 → 写入跳过提示后返回 nil，让 mise install 继续尝试
//   - apt-get update 失败 → 记一条 warning，不中断（往往只是部分镜像超时）
//   - apt-get install 失败 → 返回 error，installRuntime 据此把状态置为 failed
func (s *Service) ensureBuildDeps(ctx context.Context, lang string, log *task.TaskLog) error {
	pkgs, ok := buildDepsApt[lang]
	if !ok {
		return nil
	}
	if !hasAptGet() {
		log.Append("非 Debian/Ubuntu 系统，跳过自动安装编译依赖。如失败请手动安装：" + strings.Join(pkgs, " "))
		return nil
	}

	// apt-get update：包索引过期时 install 会报 "Unable to locate package"。
	// -qq 静默普通进度。失败不中断——常见原因是个别镜像超时，后续 install
	// 仍可用本地缓存的索引完成。Stdout/Stderr 直连 TaskLog 实时流（exec.Cmd
	// 合并两路，CombinedOutput 同款机制）。
	updateCmd := exec.CommandContext(ctx, "/usr/bin/apt-get", "update", "-qq")
	updateCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	updateCmd.Stdout = log
	updateCmd.Stderr = updateCmd.Stdout
	if err := updateCmd.Run(); err != nil {
		log.Append(fmt.Sprintf("⚠ apt-get update 失败 (%v)，继续尝试 install", err))
	}

	// -y 跳过确认；-q 减噪。已装的包会被 apt 当成"已是最新"快速跳过，
	// 因此对老服务器和新服务器是同一份代码。DEBIAN_FRONTEND=noninteractive
	// 防止撞上 tzdata 这类会忽略 -y 的交互式 postinst 直接挂住。
	args := append([]string{"install", "-y", "-q"}, pkgs...)
	installCmd := exec.CommandContext(ctx, "/usr/bin/apt-get", args...)
	installCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	installCmd.Stdout = log
	installCmd.Stderr = installCmd.Stdout
	if err := installCmd.Run(); err != nil {
		stdlog.Printf("runtime: apt-get install %s failed: %v", lang, err)
		return fmt.Errorf("apt-get install %s 失败: %w", lang, err)
	}
	return nil
}
