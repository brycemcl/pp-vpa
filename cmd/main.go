/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command pp-vpa is the single-binary entrypoint that dispatches on --mode
// into either the manager (control-plane) or the node-agent (DaemonSet)
// code path.
package main

import (
	"flag"
	"fmt"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/brycemclachlan/pp-vpa/internal/modes"
)

func main() {
	var (
		mode                 string
		metricsAddr          string
		probeAddr            string
		webhookCertPath      string
		enableLeaderElection bool
		enableWebhooks       bool
		nodeName             string
		cgroupRoot           string
		kubeletConfig        string
	)
	flag.StringVar(&mode, "mode", "manager", "Run mode: manager or node-agent.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "Metrics bind address (manager mode).")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe bind address.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "Webhook certificate directory.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election (manager mode).")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", true, "Enable mutating webhook (manager mode).")
	flag.StringVar(&nodeName, "node-name", "", "Node name (node-agent mode). Falls back to NODE_NAME env.")
	flag.StringVar(&cgroupRoot, "cgroup-root", "/sys/fs/cgroup", "Cgroup v2 root (node-agent mode).")
	flag.StringVar(&kubeletConfig, "kubelet-config", "/var/lib/kubelet/config.yaml", "Kubelet config (node-agent mode).")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	switch mode {
	case "manager":
		if err := modes.RunManager(modes.ManagerOptions{
			MetricsAddr:          metricsAddr,
			ProbeAddr:            probeAddr,
			WebhookCertDir:       webhookCertPath,
			EnableLeaderElection: enableLeaderElection,
			EnableWebhooks:       enableWebhooks,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "manager: %v\n", err)
			os.Exit(1)
		}
	case "node-agent":
		if err := modes.RunNodeAgent(modes.NodeAgentOptions{
			NodeName:      nodeName,
			CgroupRoot:    cgroupRoot,
			KubeletConfig: kubeletConfig,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "node-agent: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown --mode %q (want: manager | node-agent)\n", mode)
		os.Exit(2)
	}
}
