# **Per-Pod Vertical Pod Autoscaler (PP-VPA) Architecture**

This document outlines the architecture for a replacement of the upstream Kubernetes Vertical Pod Autoscaler (VPA). The PP-VPA provides per-pod recommendations and in-place updates, driven by Pressure Stall Information (PSI) metrics for scale-ups and utilization metrics for scale-downs.

It natively supports both container-level and pod-level resource management, and gracefully handles in-place update limitations (KEP-1287 constraints, Node policies) with robust fallback eviction strategies modeled after native Kubernetes rolling updates.

## **1\. Core Principles**

* **Edge-Actuated In-Place Updates:** High-frequency telemetry gathering and in-place Pod patching are handled entirely on the node by DaemonSets. This provides zero-latency reaction to PSI spikes and protects the API Server from watch-storms.  
* **Dual-Signal Scaling with Decaying Watermarks:** Uses PSI for rapid scale-ups, recording the usage at that moment as a dynamic high-watermark. Scale-downs use a relative drop from these watermarks based on actual utilization. Crucially, memory downscales are strictly bound by the historical peak plus a safety margin, preventing crash loops.  
* **State Inheritance & Safe Evictions:** Safely cycles pods during node-exhaustion using a budgeted "scale-up, then evict" pattern. To prevent cold-starts and starvation, new surge pods are injected with the workload's upperBound recommendation, giving them immediate headroom.  
* **Decaying Histogram Checkpointing:** Both node-local agents and the aggregate controller maintain decaying in-memory histograms of resource usage. These are periodically checkpointed to the API server to prevent autoscaling "amnesia" during controller restarts.

## **2\. API Design (CRDs)**

### **A. PerPodVerticalPodAutoscaler (The Controller)**

Targets a workload controller and defines scaling policies. It acts as the scale target for the HPA, isolating the HPA from temporary replica fluctuations.

apiVersion: autoscaling.brycemclachlan.me/v1alpha1  
kind: PerPodVerticalPodAutoscaler  
metadata:  
  name: web-app-ppvpa  
  namespace: prod  
spec:  
  targetRef:  
    apiVersion: apps/v1  
    kind: Deployment  
    name: web-app  
    
  updatePolicy:  
    updateMode: InPlace   
    anomalyEvictionTimeoutSeconds: 900 \# Evict pods that continuously exceed the aggregate upperBound (15m)  
    significantChangeThresholdPercentage: 10 \# Only update PRR if recommendation changes by \>10%  
    infeasibleUpdateBehavior:  
      maxSurge: 25%       \# Allow bursting extra temporary pods to inherit state safely (Int or String)  
      maxUnavailable: 0   \# Prevent direct hard-evictions that cause downtime (Int or String)  
    burstTimeoutSeconds: 600     \# Abort eviction if the surge pod is stuck Pending (10m)

  resourcePolicy:  
    podLevelPolicies:  
      \- resourceName: memory  
        minAllowed: 512Mi  
        maxAllowed: 16Gi  
    containerPolicies:  
      \- containerName: '\*'  
        minAllowed: { cpu: 100m }  
        maxAllowed: { cpu: "4" }

  recommenderPolicy:  
    targetPercentile: 90.0  
    lowerBoundPercentile: 50.0  
    upperBoundPercentile: 95.0  
    safetyMarginPercentage: 15 \# The Margin Estimator: Adds a static buffer to calculated bounds and enforces the minimum memory peak floor

  scalingThresholds:  
    cpu:  
      scaleUpPSI: 10.0  
      scaleDownBufferDropPercentage: 20  
    memory:  
      scaleUpPSI: 5.0  
      watermarkDecayWindowHours: 24 \# Slowly lower the high-watermark if not hit within 24h  
        
status:  
  defaultRecommendation:  
    podLevelTarget: { memory: 2Gi }  
    containerRecommendations:  
      \- containerName: web-app  
        lowerBound: { cpu: 200m }  
        target: { cpu: 500m }  
        uncappedTarget: { cpu: 550m }   
        upperBound: { cpu: 800m }

  targetReplicas: 5   
  activeReplicas: 5   
  temporaryReplicas: 0 

### **B. PodResourceRecommendation (The Child State & Local Checkpoint)**

Every pod gets an associated PRR. It acts as a persistent state-store and local histogram checkpoint.

apiVersion: autoscaling.brycemclachlan.me/v1alpha1  
kind: PodResourceRecommendation  
metadata:  
  name: web-app-7b5-prr   
  ownerReferences:  
    \- apiVersion: autoscaling.brycemclachlan.me/v1alpha1  
      kind: PerPodVerticalPodAutoscaler  
      name: web-app-ppvpa  
spec:  
  targetPodName: web-app-7b5x  
status:  
  contentionHighWatermarks:  
    podLevel: { memory: 3.5Gi, lastUpdated: "2026-05-18T12:00:00Z" }  
  observedPeak:  
    podLevel: { memory: 4Gi }  
  podLevelTarget: { memory: 4.6Gi }  
  containerRecommendations:  
    \- containerName: web-app  
      target: { cpu: 2 }   
      uncappedTarget: { cpu: 2 }  
  histogramCheckpoint: "eA1B2C3D..." 

### **C. PerPodVerticalPodAutoscalerCheckpoint (The Workload Checkpoint)**

Stores the aggregate decaying histogram for the entire workload, matching the upstream VPA Checkpoint pattern to avoid bloating the main PP-VPA object.

apiVersion: autoscaling.brycemclachlan.me/v1alpha1  
kind: PerPodVerticalPodAutoscalerCheckpoint  
metadata:  
  name: web-app-ppvpa-checkpoint  
  ownerReferences:  
    \- apiVersion: autoscaling.brycemclachlan.me/v1alpha1  
      kind: PerPodVerticalPodAutoscaler  
      name: web-app-ppvpa  
status:  
  aggregateHistogramCheckpoint: "fE4D5C6B..."

## **3\. Architecture Components**

### **Part I: Node-Local Architecture (DaemonSet)**

#### **A. The Local Patcher & Recommender**

* **State Recovery:** Decodes histogramCheckpoint from the PRR to prevent scale-down thrashing.  
* **Environmental Profiling:** Detects Node capabilities (Swap, Static CPU Manager) to flag fallback behavior.  
* **Bifurcated Metric Ingestion:**  
  * *CPU:* Continuous decaying stream.  
  * *Memory:* **Interval peak extraction** (stores only the absolute highest memory peak per defined window).  
* **OOM Bump-Up Mechanism:** On OOMKilled, injects synthetic memory sample (![][image1]).  
* **Dual-Trigger Scale-Up:** Reactive (PSI) \+ Proactive (utilization vs. requests).  
* **High-Watermark & Peak-Bound Scale-Down:**  
  * *Memory:* Strictly bounded by observedPeak \+ safetyMarginPercentage.  
* **Patch Ordering & Server-Side Congestion Control:**  
  * Pending patches are drained in priority order by |ΔCPU / NodeTotalCPU| \+ |ΔMemory / NodeTotalMem| so the largest-impact resizes go first. This is in-agent ordering, not congestion control.  
  * Congestion control and per-node fairness are delegated to Kubernetes API Priority and Fairness (APF). The node-agent ships no client-side rate limiter; see §5.

### **Part II: Cluster Control Plane (Deployments)**

#### **B. The Aggregate Recommender (Baseline Generator)**

* **Aggregation:** Merges PRR telemetry into a single, workload-wide decaying histogram.  
* **Establishing the "Safe Zone":** Applies configured percentiles for target/bounds.  
* **History-Based Confidence Multipliers:** Scaled by ![][image2].  
* **The Margin Estimator:** Applies safetyMarginPercentage as a static pad on all bounds.  
* **UncappedTarget:** Exposes the pure calculation, allowing SREs to spot truncated limits.  
* **Anomaly Detection:** If pod usage exceeds aggregate upperBound for longer than anomalyEvictionTimeoutSeconds, it flags the PRR for eviction.

#### **C. The Controller Manager (Scaling, Eviction & GC)**

* **HPA Isolation:** Reconciles Deployment to targetReplicas \+ temporaryReplicas.  
* **Anomaly Eviction:** Watches for Anomalous conditions on PRRs.  
* **Infeasible Eviction Handling (Budgeted Strategy):** Uses maxSurge and maxUnavailable (IntOrString) budgets.  
  * **Surge Path:** Adds \+1 temporaryReplica, waits for Ready, then evicts.  
  * **Unavailable Path:** Uses the **Kubernetes Eviction API** to honor PDBs.  
* **Garbage Collection:** Deletes orphaned PRRs.

#### **D. The Admission Controller (Mutating Webhook)**

* **Smart Injection:** If temporaryReplicas \> 0 OR any sibling PRRs are Infeasible or Anomalous, injects the upperBound to give replacement pods headroom. Otherwise, injects the target \+ MarginEstimator padding.

## **4\. Ecosystem Integration**

### **Horizontal Pod Autoscaler (HPA) Integration**

The HPA targets the PerPodVerticalPodAutoscaler object. It adjusts status.targetReplicas, while the PP-VPA manages the underlying Deployment replicas, isolating the HPA from temporary maxSurge inflation. The PerPodVerticalPodAutoscaler CRD exposes a /scale subresource (specReplicasPath=.status.targetReplicas, statusReplicasPath=.status.activeReplicas), which is the actual mechanism the HPA uses to target it.

### **Pod Disruption Budgets (PDB) Integration**

The PP-VPA uses the native **Kubernetes Eviction API** rather than standard DELETE. This ensures that if an eviction violates a PDB, the API rejects it, and the PP-VPA controller waits until the disruption budget replenishes.

## **5\. API Priority and Fairness Integration**

The node-agent DaemonSet generates the hottest traffic in the system: every node submits PATCH calls against `pods/<name>/resize` and writes PRR status checkpoints. Without classification, this traffic competes with default user-facing flows on `workload-low` and can be throttled or starve other clients. PP-VPA ships two `FlowSchema` + `PriorityLevelConfiguration` pairs (rendered under `config/apf/` and packaged by the Helm chart, gated by `values.apf.enabled`).

### **A. `pp-vpa-control-plane`**

* **PriorityLevelConfiguration:** `Limited`, `assuredConcurrencyShares: 30`, queued (`queueLengthLimit: 100`, `queues: 32`, `handSize: 6`). Controller-manager work is bursty (reconcile-on-event) but bounded by replica counts; queueing absorbs surges.  
* **FlowSchema:** `matchingPrecedence: 800`, `distinguisherMethod.type: ByUser`. Subjects: `ServiceAccount pp-vpa-system/pp-vpa-manager`. Verbs cover the three CRDs plus `pods`, `pods/eviction`, `deployments/scale`, `events`.

### **B. `pp-vpa-node-agents`**

* **PriorityLevelConfiguration:** `Limited`, `assuredConcurrencyShares: 50`, queued (`queueLengthLimit: 200`, `queues: 128`, `handSize: 8`). Traffic scales O(nodes); wider shuffle-sharding fan-out so one chatty node-agent cannot starve the others.  
* **FlowSchema:** `matchingPrecedence: 850`, `distinguisherMethod.type: ByUser` — **the load-bearing line.** Inside the shared priority level, APF fair-queues by user identity. Each node-agent pod runs with its own bound ServiceAccount token, shuffle-sharded into its own queue. Result: one runaway node-agent (e.g., a node with continuous PSI spikes) cannot starve other nodes; all nodes get fair access to the level's concurrency budget. Subjects: `ServiceAccount pp-vpa-system/pp-vpa-node-agent`. Writes: `patch`/`update` on `pods/resize`, `podresourcerecommendations/status`. Reads: `get`/`list`/`watch` on `pods`, `podresourcerecommendations`, `perpodverticalpodautoscalers`, colocated in the same flow so reads can't be throttled separately during write-heavy phases.

### **C. Why two levels rather than one**

Isolating control-plane work from node-agent work prevents a runaway DaemonSet (e.g., a bad recommender rollout patching every pod) from blocking the controller's ability to react. Each level has its own queue budget; saturation in one does not bleed into the other.

[image1]: <data:image/png;base64,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>

[image2]: <data:image/png;base64,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>