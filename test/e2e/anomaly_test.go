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

var _ = Describe("Anomaly Detection", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(5 * time.Second)

	const deployName = "anomaly-test"
	const ppvpaName = "anomaly-test"

	AfterAll(func() {
		cleanupPPVPA(ppvpaName)
		cleanupDeployment(deployName)
	})

	It("should create a PRR for each pod", func() {
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

		By("waiting for PRRs to be created")
		Eventually(func() int {
			out, _ := exec.Command("kubectl", "get", "prr", "-n", "default",
				"-o", "jsonpath={.items[*].metadata.name}").CombinedOutput()
			names := strings.Fields(string(out))
			count := 0
			for _, n := range names {
				if strings.Contains(n, ppvpaName) {
					count++
				}
			}
			return count
		}).Should(BeNumerically(">=", 2))
	})

	It("should populate PPVPA status with recommendations", func() {
		By("waiting for the PPVPA status to have a recommendation")
		Eventually(func() string {
			out, _ := exec.Command("kubectl", "get", "ppvpa", ppvpaName, "-n", "default",
				"-o", "jsonpath={.status.defaultRecommendation}").CombinedOutput()
			return string(out)
		}).ShouldNot(BeEmpty())
	})

	It("should report activeReplicas matching pod count", func() {
		By("checking activeReplicas in PPVPA status")
		Eventually(func() string {
			out, _ := exec.Command("kubectl", "get", "ppvpa", ppvpaName, "-n", "default",
				"-o", "jsonpath={.status.activeReplicas}").CombinedOutput()
			return strings.TrimSpace(string(out))
		}).Should(Equal("2"))
	})
})
