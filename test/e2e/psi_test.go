//go:build e2e
// +build e2e

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/brycemclachlan/pp-vpa/test/utils"
)

var _ = Describe("PSI Scale-Up", Ordered, func() {
	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	var workerNode string

	BeforeAll(func() {
		workerNode = getWorkerNode()
		Expect(workerNode).NotTo(BeEmpty(), "expected at least one worker node")

		if !checkPSIAvailable(workerNode) {
			Skip("PSI not available -- requires Linux kernel 4.20+ with CONFIG_PSI=y and cgroup v2")
		}
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			dumpPSIDiagnostics()
		}
	})

	Context("CPU pressure", func() {
		const deployName = "psi-test-cpu"
		const ppvpaName = "psi-test-cpu"

		AfterAll(func() {
			cleanupPPVPA(ppvpaName)
			cleanupDeployment(deployName)
		})

		It("should trigger in-place CPU scale-up when a pod is throttled", func() {
			By("creating a CPU-stressed deployment")
			createDeployment(deployName, workerNode,
				"registry.k8s.io/e2e-test-images/agnhost:2.47",
				[]string{"stress", "--cpus", "1"},
				"100m", "256Mi")

			By("creating a PPVPA with a low CPU PSI threshold")
			createPPVPA(ppvpaName, deployName, "5.0", "")

			By("waiting for a PRR to be created")
			Eventually(func() string {
				return waitForPRR(ppvpaName)
			}, 2*time.Minute, 2*time.Second).ShouldNot(BeEmpty())

			By("waiting for the pod's CPU limit to increase")
			Eventually(func() string {
				return getPodCPULimit(deployName)
			}, 3*time.Minute, 5*time.Second).ShouldNot(Equal("100m"))

			By("verifying the new CPU is greater than 100m")
			newCPU := getPodCPULimit(deployName)
			Expect(parseMilliCores(newCPU)).To(BeNumerically(">", 100))
		})
	})

	Context("Memory pressure", func() {
		const deployName = "psi-test-mem"
		const ppvpaName = "psi-test-mem"

		AfterAll(func() {
			cleanupPPVPA(ppvpaName)
			cleanupDeployment(deployName)
		})

		It("should trigger in-place memory scale-up when memory is pressured", func() {
			By("creating a memory-stressed deployment")
			createDeployment(deployName, workerNode,
				"registry.k8s.io/e2e-test-images/agnhost:2.47",
				[]string{"stress", "--mem-alloc-size", "200"},
				"100m", "200Mi")

			By("creating a PPVPA with a low memory PSI threshold")
			createPPVPA(ppvpaName, deployName, "", "5.0")

			By("waiting for a PRR to be created")
			Eventually(func() string {
				return waitForPRR(ppvpaName)
			}, 2*time.Minute, 2*time.Second).ShouldNot(BeEmpty())

			By("waiting for the pod's memory limit to increase")
			Eventually(func() string {
				return getPodMemLimit(deployName)
			}, 3*time.Minute, 5*time.Second).ShouldNot(Equal("200Mi"))

			By("verifying the new memory is greater than 200Mi")
			newMem := getPodMemLimit(deployName)
			Expect(parseQuantity(newMem)).To(BeNumerically(">", 200*1024*1024))
		})
	})
})

// getWorkerNode returns the first worker node name.
func getWorkerNode() string {
	cmd := exec.Command("kubectl", "get", "nodes",
		"-o", "jsonpath={.items[0].metadata.name}")
	out, err := utils.Run(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// checkPSIAvailable queries cadvisor for container PSI metrics.
func checkPSIAvailable(node string) bool {
	cmd := exec.Command("kubectl", "get",
		"--raw", fmt.Sprintf("/api/v1/nodes/%s/proxy/metrics/cadvisor", node))
	out, err := utils.Run(cmd)
	if err != nil {
		return false
	}
	return strings.Contains(out, "container_pressure_")
}

// createDeployment creates a Deployment with a single container pinned to a node.
func createDeployment(name, node, image string, args []string, cpuLimit, memLimit string) {
	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      nodeName: %s
      containers:
        - name: stress
          image: %s
          args: [%s]
          resources:
            requests:
              cpu: %s
              memory: %s
            limits:
              cpu: %s
              memory: %s
`, name, name, name, node, image,
		`"`+strings.Join(args, `", "`)+`"`,
		cpuLimit, memLimit, cpuLimit, memLimit)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// createPPVPA creates a PerPodVerticalPodAutoscaler targeting the named Deployment.
func createPPVPA(name, deployName, cpuPSI, memPSI string) {
	parts := []string{}
	if cpuPSI != "" {
		parts = append(parts, fmt.Sprintf(`cpu:
      scaleUpPSI: "%s"`, cpuPSI))
	}
	if memPSI != "" {
		parts = append(parts, fmt.Sprintf(`memory:
      scaleUpPSI: "%s"`, memPSI))
	}
	thresholds := strings.Join(parts, "\n    ")

	yaml := fmt.Sprintf(`apiVersion: autoscaling.brycemclachlan.me/v1alpha1
kind: PerPodVerticalPodAutoscaler
metadata:
  name: %s
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: %s
  updatePolicy:
    updateMode: InPlace
  resourcePolicy:
    containerPolicies:
      - containerName: "*"
        maxAllowed:
          cpu: "2"
          memory: "2Gi"
  scalingThresholds:
    %s
`, name, deployName, thresholds)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// waitForPRR waits for a PRR owned by the named PPVPA and returns the PRR name.
func waitForPRR(ppvpaName string) string {
	cmd := exec.Command("kubectl", "get", "prr",
		"-o", fmt.Sprintf(
			`jsonpath={range .items[?(@.metadata.ownerReferences[0].name=="%s")]}{.metadata.name}{end}`, ppvpaName))
	out, err := utils.Run(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// getPodCPULimit returns the CPU limit of the first container in the deployment's pod.
func getPodCPULimit(deployName string) string {
	cmd := exec.Command("kubectl", "get", "pods",
		"-l", "app="+deployName,
		"-o", "jsonpath={.items[0].spec.containers[0].resources.limits.cpu}")
	out, err := utils.Run(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// getPodMemLimit returns the memory limit of the first container in the deployment's pod.
func getPodMemLimit(deployName string) string {
	cmd := exec.Command("kubectl", "get", "pods",
		"-l", "app="+deployName,
		"-o", "jsonpath={.items[0].spec.containers[0].resources.limits.memory}")
	out, err := utils.Run(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// parseMilliCores parses a Kubernetes CPU quantity string and returns milliCores.
func parseMilliCores(s string) int64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "m") {
		var val int64
		fmt.Sscanf(s, "%dm", &val)
		return val
	}
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return int64(val * 1000)
}

// parseQuantity parses a Kubernetes memory quantity string and returns bytes.
func parseQuantity(s string) int64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "Ki") {
		var val float64
		fmt.Sscanf(s, "%fKi", &val)
		return int64(val * 1024)
	}
	if strings.HasSuffix(s, "Mi") {
		var val float64
		fmt.Sscanf(s, "%fMi", &val)
		return int64(val * 1024 * 1024)
	}
	if strings.HasSuffix(s, "Gi") {
		var val float64
		fmt.Sscanf(s, "%fGi", &val)
		return int64(val * 1024 * 1024 * 1024)
	}
	var val int64
	fmt.Sscanf(s, "%d", &val)
	return val
}

// cleanupDeployment deletes a Deployment and waits for pods to vanish.
func cleanupDeployment(name string) {
	cmd := exec.Command("kubectl", "delete", "deployment", name, "--ignore-not-found")
	_, _ = utils.Run(cmd)
	Eventually(func() string {
		cmd := exec.Command("kubectl", "get", "pods", "-l", "app="+name, "-o", "jsonpath={.items}")
		out, _ := utils.Run(cmd)
		return out
	}, 30*time.Second).Should(BeEmpty())
}

// cleanupPPVPA deletes a PPVPA.
func cleanupPPVPA(name string) {
	cmd := exec.Command("kubectl", "delete", "ppvpa", name, "--ignore-not-found")
	_, _ = utils.Run(cmd)
}

// dumpPSIDiagnostics logs diagnostic information on test failure.
func dumpPSIDiagnostics() {
	By("Fetching node-agent logs")
	cmd := exec.Command("kubectl", "logs", "ds/pp-vpa-node-agent", "-n", namespace, "--tail=200")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Node-agent logs:\n%s\n", out)
	}

	By("Fetching PPVPA status")
	cmd = exec.Command("kubectl", "get", "ppvpa", "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "PPVPA:\n%s\n", out)
	}

	By("Fetching PRR status")
	cmd = exec.Command("kubectl", "get", "prr", "-o", "yaml")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "PRR:\n%s\n", out)
	}

	By("Fetching pod descriptions")
	cmd = exec.Command("kubectl", "get", "pods", "-o", "name")
	if out, err := utils.Run(cmd); err == nil {
		for _, podName := range utils.GetNonEmptyLines(out) {
			if strings.Contains(podName, "psi-test") {
				cmd := exec.Command("kubectl", "describe", podName)
				if desc, err := utils.Run(cmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Pod %s:\n%s\n", podName, desc)
				}
			}
		}
	}
}
