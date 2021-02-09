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

// Package clc generates Ignition using Container Linux Config Transpiler.
package clc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	ignition "github.com/coreos/ignition/config/v2_3"
	ignitionTypes "github.com/coreos/ignition/config/v2_3/types"
	clct "github.com/flatcar-linux/container-linux-config-transpiler/config"
	"github.com/pkg/errors"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/cloudinit"
)

const (
	clcTemplate = `---
systemd:
  units:
    - name: kubeadm.service
      enabled: true
      contents: |
        [Unit]
        Description=kubeadm
        [Service]
        # To not restart the unit when it exits, as it is expected.
        Type=oneshot
        ExecStart=/etc/kubeadm.sh
        [Install]
        WantedBy=multi-user.target

storage:
  files:{{ range .WriteFiles }}
  - path: {{.Path}}
    {{ if ne .Permissions "" -}}
    mode: {{.Permissions}}
    {{ end -}}
    contents:
      inline: |
{{.Content | Indent 8}}
{{- end }}
  - path: /etc/kubeadm.sh
    mode: 0700
    contents:
      inline: |
        #!/bin/bash
        set -e

        {{- range .PreKubeadmCommands }}
        {{ . }}
        {{- end }}

        {{ .KubeadmCommand }}
        mv /etc/kubeadm.yml /tmp/

        {{- range .PostKubeadmCommands }}
        {{ . }}
        {{- end }}
  - path: /etc/kubeadm.yml
    mode: 0600
    contents:
      inline: |
        ---
{{ .KubeadmConfig | Indent 8 }}
`
)

type render struct {
	*cloudinit.BaseUserData

	KubeadmConfig string
}

func defaultTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"Indent": templateYAMLIndent,
	}
}

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)
	return strings.Repeat(" ", i) + strings.Join(split, ident)
}

func renderCLC(input *cloudinit.BaseUserData, kubeadmConfig string) ([]byte, error) {
	if input == nil {
		return nil, errors.New("empty base user data")
	}

	t := template.Must(template.New("template").Funcs(defaultTemplateFuncMap()).Parse(clcTemplate))

	data := render{
		BaseUserData:  input,
		KubeadmConfig: kubeadmConfig,
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render template")
	}

	return out.Bytes(), nil
}

func Render(input *cloudinit.BaseUserData, clc *bootstrapv1.ContainerLinuxConfig, kubeadmConfig string) ([]byte, string, error) {
	if clc == nil {
		return nil, "", errors.New("get empty CLC config")
	}

	clcBytes, err := renderCLC(input, kubeadmConfig)
	if err != nil {
		return nil, "", errors.Wrapf(err, "rendering CLC configuration")
	}

	userData, warnings, err := buildIgnitionConfig(clcBytes, clc)
	if err != nil {
		return nil, "", errors.Wrapf(err, "building Ignition config")
	}

	return userData, warnings, nil
}

func buildIgnitionConfig(baseCLC []byte, clc *bootstrapv1.ContainerLinuxConfig) ([]byte, string, error) {
	// We control baseCLC config, so treat it as strict.
	ign, _, err := clcToIgnition(baseCLC, true)
	if err != nil {
		return nil, "", errors.Wrapf(err, "converting generated CLC to Ignition")
	}

	var clcWarnings string

	if clc.AdditionalConfig != "" {
		additionalIgn, warnings, err := clcToIgnition([]byte(clc.AdditionalConfig), clc.Strict)
		if err != nil {
			return nil, "", errors.Wrapf(err, "converting additional CLC to Ignition")
		}

		clcWarnings = warnings

		ign = ignition.Append(ign, additionalIgn)
	}

	userData, err := json.Marshal(&ign)
	if err != nil {
		return nil, "", errors.Wrapf(err, "marshaling generated Ignition config into JSON")
	}

	return userData, clcWarnings, nil
}

func clcToIgnition(data []byte, strict bool) (ignitionTypes.Config, string, error) {
	clc, ast, reports := clct.Parse(data)

	if (len(reports.Entries) > 0 && strict) || reports.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error parsing Container Linux Config: %v", reports.String())
	}

	ign, report := clct.Convert(clc, "", ast)
	if (len(report.Entries) > 0 && strict) || report.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error converting to Ignition: %v", report.String())
	}

	reports.Merge(report)

	return ign, reports.String(), nil
}
