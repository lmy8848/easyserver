package runtimeenv

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

// buildDepsApt 列出在 Debian/Ubuntu 上从源码编译指定运行时所需的系统包。
// asdf-php、python-build 都是 source build——必须先把工具链装好，否则
// mise install 会停在 buildconf/configure 阶段并把"autoconf not found"
// 之类的错抛给用户。
//
// node / go / java 都用预编译二进制，无需本地工具链，因此不在此表。
var buildDepsApt = map[string][]string{
	"php": {
		"build-essential", "autoconf", "bison", "re2c", "pkg-config",
		"libxml2-dev", "libssl-dev", "libicu-dev", "libsqlite3-dev",
		"libcurl4-openssl-dev", "libonig-dev", "libzip-dev", "zlib1g-dev",
		"libgd-dev", "libpq-dev", "libbz2-dev", "libjpeg-dev", "libpng-dev",
		"libreadline-dev", "libtidy-dev", "libxslt1-dev",
	},
	"python": {
		"build-essential", "libssl-dev", "zlib1g-dev", "libbz2-dev",
		"libreadline-dev", "libsqlite3-dev", "libffi-dev", "liblzma-dev",
		"pkg-config", "tk-dev", "libncursesw5-dev", "xz-utils",
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
// 整个过程的输出由 runStreaming 流式写入运行环境的安装日志，前端"查看
// 日志"可见。
//
// 行为：
//   - lang 不在 buildDepsApt 中（node/go/java）→ 直接返回 nil
//   - 非 apt 系统 → 写入跳过提示后返回 nil，让 mise install 继续尝试
//   - apt-get update 失败 → 记一条 warning，不中断（往往只是部分镜像超时）
//   - apt-get install 失败 → 返回 error，installRuntime 据此把状态置为 failed
func (s *Service) ensureBuildDeps(ctx context.Context, id int64, lang string) error {
	pkgs, ok := buildDepsApt[lang]
	if !ok {
		return nil
	}
	if !hasAptGet() {
		s.appendProgress(ctx, id, 10, "deps-skip",
			fmt.Sprintf("非 Debian/Ubuntu 系统，跳过自动安装编译依赖。如失败请手动安装：%s",
				strings.Join(pkgs, " ")))
		return nil
	}

	// apt-get update：包索引过期时 install 会报 "Unable to locate package"。
	// -qq 静默普通进度。失败不中断——常见原因是个别镜像超时，后续 install
	// 仍可用本地缓存的索引完成。
	if _, exitCode, err := s.runStreaming(ctx, id, 5, "apt-update",
		"更新 apt 软件包索引 (apt-get update)",
		"/usr/bin/apt-get", "update", "-qq"); err != nil || exitCode != 0 {
		s.appendProgress(ctx, id, 10, "apt-update",
			fmt.Sprintf("⚠ apt-get update 失败 (exit %d, err=%v)，继续尝试 install", exitCode, err))
	}

	// -y 跳过确认；-q 减噪。已装的包会被 apt 当成"已是最新"快速跳过，
	// 因此对老服务器和新服务器是同一份代码。
	args := append([]string{"install", "-y", "-q"}, pkgs...)
	output, exitCode, err := s.runStreaming(ctx, id, 15, "apt-install",
		fmt.Sprintf("安装 %s 编译依赖 (%d 个包: %s)", lang, len(pkgs), strings.Join(pkgs, " ")),
		"/usr/bin/apt-get", args...)
	if err != nil || exitCode != 0 {
		log.Printf("runtime: apt-get install %s failed (exit %d): %v", lang, exitCode, err)
		return fmt.Errorf("apt-get install %s 失败 (exit %d): %s",
			lang, exitCode, tailLines(output, 8))
	}
	return nil
}
