/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package clc_test tests clc package.
package clc_test

import (
	"testing"

	ignition "github.com/coreos/ignition/config/v2_3"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/cloudinit"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/ignition/clc"
)

const configWithWarning = `---
storage:
  files:
  - path: /foo
    contents:
      inline: foo
`

func Test_Render(t *testing.T) {
	t.Parallel()

	t.Run("renders_valid_Ignition_JSON", func(t *testing.T) {
		t.Parallel()

		input := &cloudinit.BaseUserData{}
		config := &bootstrapv1.ContainerLinuxConfig{}
		kubeadmConfig := "foo"

		ignitionBytes, _, err := clc.Render(input, config, kubeadmConfig)
		if err != nil {
			t.Fatalf("rendering: %v", err)
		}

		_, reports, err := ignition.Parse(ignitionBytes)
		if err != nil {
			t.Fatalf("Parsing generated Ignition: %v", err)
		}

		if reports.IsFatal() {
			t.Fatalf("Generated Ignition has fatal reports: %s", reports)
		}
	})

	t.Run("validates_input_parameter", func(t *testing.T) {
		t.Parallel()

		if _, _, err := clc.Render(nil, &bootstrapv1.ContainerLinuxConfig{}, "foo"); err == nil {
			t.Fatal("expected error when passing empty input data")
		}
	})

	t.Run("validates_clc_parameter", func(t *testing.T) {
		t.Parallel()

		if _, _, err := clc.Render(&cloudinit.BaseUserData{}, nil, "bar"); err == nil {
			t.Fatal("expected error when passing empty CLC config")
		}
	})

	t.Run("treats_warnings_as_errors_in_strict_mode", func(t *testing.T) {
		config := &bootstrapv1.ContainerLinuxConfig{
			Strict:           true,
			AdditionalConfig: configWithWarning,
		}

		if _, _, err := clc.Render(&cloudinit.BaseUserData{}, config, "foo"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("returns_warnings", func(t *testing.T) {
		config := &bootstrapv1.ContainerLinuxConfig{
			AdditionalConfig: configWithWarning,
		}

		data, warnings, err := clc.Render(&cloudinit.BaseUserData{}, config, "foo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if warnings == "" {
			t.Errorf("expected warnings to be not empty")
		}

		if len(data) == 0 {
			t.Errorf("epected data to be returned on config with warnings")
		}
	})
}
