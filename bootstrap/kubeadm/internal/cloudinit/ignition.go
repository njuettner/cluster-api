package cloudinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/flatcar-linux/container-linux-config-transpiler/config"
	"github.com/flatcar-linux/container-linux-config-transpiler/config/platform"
	"github.com/pkg/errors"
)

const (
	baseIgnition = `---
systemd:
  units:
    - name: coreos-metadata.service
      enabled: true
      dropins:
      - name: 00-fix-enable.conf
        contents: |
          [Service]
          RemainAfterExit=true
          [Install]
          WantedBy=multi-user.target
    - name: kubeadm.service
      enabled: true
      contents: |
        [Unit]
        Description=kubeadm
        Requires=coreos-metadata.service
        After=coreos-metadata.service
        [Service]
        Type=oneshot
        RemainAfterExit=true
        Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/opt/bin
        EnvironmentFile=/run/metadata/*
        ExecStart=/etc/kubeadm.sh
        [Install]
        WantedBy=multi-user.target

storage:
  links:
  - path: /etc/systemd/system/multi-user.target.wants/coreos-metadata.service
    target: /usr/lib/systemd/system/coreos-metadata.service
  - path: /etc/systemd/system/multi-user.target.wants/kubeadm.service
    target: /etc/systemd/system/kubeadm.service
  files:{{ range .WriteFiles }}
  - path: {{.Path}}
    {{ if ne .Permissions "" -}}
    mode: {{.Permissions}}
    {{ end -}}
    contents:
      inline: |
{{.Content | Indent 8}}
{{- end }}
`
	controlPlaneIgnitionInit = `  - path: /etc/kubeadm.sh
    mode: 0700
    contents:
      inline: |
        #!/bin/bash
        set -e
        cat /etc/kubeadm.yml.tmpl | envsubst > /etc/kubeadm.yml
        kubeadm init --config /etc/kubeadm.yml
        rm /etc/kubeadm.yml /etc/kubeadm.yml.tmpl
  - path: /etc/kubeadm.yml.tmpl
    mode: 0600
    contents:
      inline: |
        ---
{{.ClusterConfiguration | Indent 8}}
        ---
{{.InitConfiguration | Indent 8}}
`
	nodeIgnitionJoin = `  - path: /etc/kubeadm.sh
    mode: 0700
    contents:
      inline: |
        #!/bin/bash
        set -e
        cat /etc/kubeadm-join-config.yaml.tmpl | envsubst > /etc/kubeadm-join-config.yaml
        kubeadm join --config /etc/kubeadm-join-config.yaml
        rm /etc/kubeadm-join-config.yaml /etc/kubeadm-join-config.yaml.tmpl
  - path: /etc/kubeadm-join-config.yaml.tmpl
    mode: 0600
    contents:
      inline: |
        ---
{{.JoinConfiguration | Indent 8}}
`
)

func NewInitControlPlaneIgnition(input *ControlPlaneInput) ([]byte, error) {
	input.WriteFiles = input.Certificates.AsFiles()

	return generateIgnition("InitControlplane", baseIgnition+controlPlaneIgnitionInit, input)
}

func NewJoinControlPlaneIgnition(input *ControlPlaneJoinInput) ([]byte, error) {
	input.WriteFiles = input.Certificates.AsFiles()

	return generateIgnition("Node", baseIgnition+nodeIgnitionJoin, input)
}

func NewNodeIgnition(input *NodeInput) ([]byte, error) {
	return generateIgnition("Node", baseIgnition+nodeIgnitionJoin, input)
}

func generateIgnition(kind string, tpl string, data interface{}) ([]byte, error) {
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)

	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	cfg, ast, report := config.Parse(out.Bytes())
	if len(report.Entries) > 0 {
		return nil, fmt.Errorf("parsing generated CLC: %v", report.String())
	}

	ignCfg, report := config.Convert(cfg, platform.EC2, ast)
	if len(report.Entries) > 0 {
		return nil, fmt.Errorf("converting parsed CLC into Ignition: %v", report.String())
	}

	userData, err := json.Marshal(&ignCfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling generated Ignition config into JSON: %w", err)
	}

	return userData, nil
}
