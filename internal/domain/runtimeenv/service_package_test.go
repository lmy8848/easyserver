package runtimeenv

import (
	"errors"
	"testing"

	"easyserver/internal/infra/errx"
)

func TestValidatePackageName(t *testing.T) {
	valid := []string{
		"express",
		"@types/node",
		"@scope/package-name",
		"requests",
		"django_rest_framework",
		"flask",
	}
	for _, name := range valid {
		if err := validatePackageName(name); err != nil {
			t.Errorf("expected valid package name %q, got error: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"-invalid",
		"/invalid",
		"@/invalid",
		"@scope/",
		"pkg;rm -rf /",
		"pkg name",
	}
	for _, name := range invalid {
		if err := validatePackageName(name); err == nil {
			t.Errorf("expected invalid package name %q to fail", name)
		} else if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest for %q, got: %v", name, err)
		}
	}
}

func TestValidatePackageVersion(t *testing.T) {
	t.Run("node/npm version ranges", func(t *testing.T) {
		valid := []string{
			"",
			"1.0.0",
			"^1.2.3",
			"~1.2.0",
			">=1.0.0 <2.0.0",
			">=1.0.0 || >=2.0.0",
			"1.0.0-beta.1",
			"1.x",
			"*",
			"latest",
		}
		for _, v := range valid {
			if err := validatePackageVersion("node", v); err != nil {
				t.Errorf("expected valid npm version %q, got: %v", v, err)
			}
		}

		invalid := []string{
			"1.0.0;rm -rf /",
			"1.0.0`ls`",
			"1.0.0$PATH",
		}
		for _, v := range invalid {
			if err := validatePackageVersion("node", v); err == nil {
				t.Errorf("expected invalid npm version %q to fail", v)
			}
		}
	})

	t.Run("python/pip exact versions", func(t *testing.T) {
		valid := []string{
			"",
			"1.0.0",
			"2.0.0a1",
			"0.1.dev1",
			"1.2.3.post1",
			"1.2.3-1",
			"1.2.3+local",
		}
		for _, v := range valid {
			if err := validatePackageVersion("python", v); err != nil {
				t.Errorf("expected valid pip version %q, got: %v", v, err)
			}
		}

		invalid := []string{
			">=1.0.0",
			">=1.0.0 <2.0.0",
			"^1.0.0",
			"~1.0.0",
			"1.0.0;rm",
		}
		for _, v := range invalid {
			if err := validatePackageVersion("python", v); err == nil {
				t.Errorf("expected invalid pip version %q to fail", v)
			}
		}
	})
}
