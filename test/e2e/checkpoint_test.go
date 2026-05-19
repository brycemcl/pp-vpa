//go:build e2e
// +build e2e

/*
Copyright 2026.

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

var _ = Describe("Checkpoint", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(5 * time.Second)

	const deployName = "checkpoint-test"
	const ppvpaName = "checkpoint-test"

	AfterAll(func() {
		cleanupPPVPA(ppvpaName)
		cleanupDeployment(deployName)
	})

	It("should create a checkpoint CR after deploying a PPVPA", func() {
		By("creating a deployment")
		utils.Run(exec.Command("kubectl", "create", "deployment", deployName,
			"--image=registry.k8s.io/pause:3.9",
			"--replicas=2", "--namespace=default"))

		By("creating a PPVPA targeting the deployment")
		ppvpaYAML := fmt.Sprintf(`
apiVersion: autoscaling.brycemclachlan.me/v1alpha1
kind: PerPodVerticalPodAutoscaler
metadata:
  name: %s
  namespace: default
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: %s
  updatePolicy:
    updateMode: InPlace
  recommenderPolicy:
    safetyMarginPercentage: 15
`, ppvpaName, deployName)
		utils.Run(exec.Command("kubectl", "apply", "-f", "-"), ppvpaYAML)

		By("waiting for a checkpoint CR to be created")
		Eventually(func() string {
			out, _ := exec.Command("kubectl", "get", "perpodverticalpodautoscalercheckpoint",
				"-n", "default", "-o", "jsonpath={.items[*].metadata.name}").CombinedOutput()
			return string(out)
		}).Should(ContainSubstring("checkpoint"))
	})

	It("should populate the checkpoint with histogram data", func() {
		By("waiting for the checkpoint to have data")
		Eventually(func() string {
			out, _ := exec.Command("kubectl", "get", "perpodverticalpodautoscalercheckpoint",
				fmt.Sprintf("%s-checkpoint", ppvpaName), "-n", "default",
				"-o", "jsonpath={.status.aggregateHistogramCheckpoint}").CombinedOutput()
			return string(out)
		}).ShouldNot(BeEmpty())
	})

	It("should have ownerReference pointing to PPVPA", func() {
		out, err := exec.Command("kubectl", "get", "perpodverticalpodautoscalercheckpoint",
			fmt.Sprintf("%s-checkpoint", ppvpaName), "-n", "default",
			"-o", "jsonpath={.metadata.ownerReferences[0].kind}").CombinedOutput()
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(string(out))).To(Equal("PerPodVerticalPodAutoscaler"))
	})
})
