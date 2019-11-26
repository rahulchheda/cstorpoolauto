## CStorCluster Data Agility Operator
- CStorCluster data agility operator will manage cstor storage lifecycle
- It will handle the following tasks:
    - Identify availabe storages
    - Provision desired storage(s)
    - Build & apply CStorPoolCluster
    - Manage cstor pool cluster's day 2 operations
- Following are various custom resources to implement the same

```yaml
# This manages the lifecycle of cstor storage
kind: CStorClusterConfig
metadata:
    name:
    namespace:
    labels:
    annotations:
spec:
    # Minimum number of cstor pool instances
    #
    # Defaults to the minimum value from below:
    # - number of allowed nodes or 
    # - number of kubernetes nodes or 
    # - 3
    minPools:
    
    # Maximum number of cstor pool instances
    #
    # Default: minPools + 2
    # Note: can be same as minimum count of pools
    maxPools:
    
    # Eligible nodes that can hold cstor pool instances
    allowedNodes:
        nodeSelectorTerms:
        -   matchLabels:
            matchAnnotations:

    # Disk details used to build cstor pool instances
    diskConfig:
        # Minimum number of disks to be used per cstor pool instance
        #
        # dependent on defaultRAIDType
        # - defaults to 2 if mirror or stripe
        # - default to 3 if raidz
        minCount:

        # Maximum number of disks to be used per cstor pool instance
        #
        # Default: How to calculate?
        # Note: Can be same as minimum count of disks
        maxCount:
        
        # capacity of each disk that participates in cstor pool instance
        #
        # defaults to 100Gi
        minCapacity:
        
        # provisioner controller used to provision cloud disks
        # 
        # auto detect !!
        externalProvisioner:
            csiAttacherName:
            storageClassName:
    poolConfig:
        # Write cache determines if all pool instances should have a write cache
        writeCache: # auto detect !!
            csiAttacherName:
            storageClassName:
        
        # Read cache determines if all pool instances should have a read cache
        readCache: # auto detect !! 
            csiAttacherName:
            storageClassName:
        
        # If all pool instances should have a spare
        spare:
        
        # Specify the RAID type i.e. mirror, stripe, raidz
        #
        # default to mirror
        defaultRAIDType:

        poolExpansion:
            disable:
            threshold:
                used-capacity: 0.80
        
        computeResources:
            requests:
                memory:
                cpu: 
            limits:
                memory:
                cpu:
status:
    # Init - controller has not kicked in
    # Error - controller resulted in some failures
    # InProgress - cstor pool creation is in progress
    # Running - cstor pool is running
    phase: 
    conditions: 
    # planning
    # replanning
    # defaulting
    # detecting
```

```yaml
# CStorClusterPlan gets created based on CStorClusterConfig and is
# used as a reference to create/update CStorPoolCluster
kind: CStorClusterPlan
metadata:
    name:
    namespace:
spec:
    poolClusterPlan:
        # Pool type to be used for all cstor pool instances
        # e.g. stripe, mirror, raidz, etc
        poolType:
        # Disk configuration to be applied across all cstor pool instances
        diskConfig:
            # Number of disks per cstor pool instance
            diskCount:
            # Capacity of each disk of a cstor pool instance
            diskCapacity:
        # Worker nodes that form a cstor pool cluster. Each node will become
        # a cstor pool instance
        nodePlanList:
        -   nodeDetail:
                # Name of the node
                nodeName:
                # UID of the node
                nodeUID:
            # Devices that form one cstor pool instance
            deviceDetailList:
            -   blockDeviceName:
status:
    phase:
    conditions:
```

```yaml
# CStorClusterRePlan is used to update CStorClusterPlan
# after satisfying CStorClusterConfig. Once CStorClusterPlan 
# is updated this resource is purged.
kind: CStorClusterRePlan
metadata:
    name:
    namespace:
spec:
    # A new node i.e. new cstor pool instance is added to the
    # existing cstor pool cluster
    nodeAdd:
    -   nodeName:
    # Remove an existing node i.e. an existing cstor pool instance
    # from the cstor pool cluster
    nodeRemove:
    -   nodeName:
    # Node replacement will detach the disks from the current
    # node and attach them to the target node
    nodeReplace:
    -   currentNodeName:
        targetNodeName:
    # Device replacement will detach the current device and
    # attach the target device in its place
    deviceReplace:
    -   nodeName:
        currentDeviceName:
        targetDeviceName:
    # Disk resize will resize the given device to the given
    # capacity.
    diskResize:
    -   nodeName:
        deviceName:
        diskCapacity:
status:
    phase:
    conditions:
```

## High Level Design & Workflows
### Workflow for creation of pool cluster

### Workflow for deletion of pool cluster

### Workflow for modifying the max pools

### Workflow when node selectors get changed at runtime
This workflow provides the high level implementation details when node labels, selector gets changed at runtime. In other words, how CSPCAuto handles changes to node label selector that has impact to this CSPCAuto reconciliation mechanism.

### Workflow when one of nodes running the cstor pool is taken out of cluster

## Known Issues
### Correlate block device with volume attachment. 
Block devices may be created via multiple ways. One of ways to have a BlockDevice created is via VolumeAttachment. We do not have a concrete way to map a block device with volume attachment even if the block device was created due to the attachment.

>> let storage code watch for BDC and with annotation, and NDM will not watch for it __ @vitta

## Deployment Strategy
CStor Pool Auto controller will consist of following deployments:

### Core Deployment (a K8s Pod)
Core deployment implies the actual business logic. It will consist of following components/containers:
- cstorpoolauto (a http service)
- cstorpoolauto-controller (a http client & kubernetes controller)

#### 1/ cstorpoolauto (a K8s sidecar i.e. container)
cstorpoolauto implements business logic to auto provision cstor pool(s). This is deployed as a http service that exposes one or more http endpoints. These endpoints get invoked by cstorpoolauto controller.

#### 2/ cstorpoolauto-controller (a K8s sidecar i.e. container)
cstorpoolauto controller watches Kubernetes resource(s) and invokes appropriate http endpoints exposed by cspauto service. In other words, this is the http client for cstorpoolauto service & at the same time is a watcher of Kubernetes resource(s).

### Conformance Deployment (optional) (a K8s Pod)
Conformance deployment has the conformance logic to verify if cspauto service is working as expected. This is completely optional and can be installed or un-installed without any disruptions to the core services. Conformance deployment will consist of following components:
- cstorpoolauto-conformance (a conformance http service)
- cstorpoolauto-conformance-controller (a http client & kubernetes controller)

#### 1/ cstorpoolauto-conformance (a K8s sidecar i.e. container)
cstorpoolauto conformance implements conformance logic to verify if cstorpoolauto service is functioning properly. This is deployed as a http service. This exposes one or more http endpoints that get invoked by cstorpoolauto conformance controller.

#### 2/ cstorpoolauto-conformance-controller (a K8s sidecar i.e. container)
cstorpoolauto conformance controller watches Kubernetes resource(s) and invokes appropriate http endpoints exposed by cstorpoolauto conformance service. In other words, this is the http client for cstorpoolauto conformance service & at the same time is a watcher of Kubernetes resource(s).

## Old Design(s)
### Design 1
This design was the first attempt to get around the difficulties faced to create a CSPC yaml. In addition, it took care of creating & attaching un-available disks 

```yaml
kind: CSPCAuto
metadata:
    name:
    annotations:
        # These annotations that determine the storage provider.
        # They are required to provision storage if required.
        dao.mayadata.io/storage-class-name: 
        dao.mayadata.io/csi-driver-name:
spec:
    # desired state for each pool instance
    cspiList:
        # pool type e.g. either stripe, mirror, or raidz to 
        # configure each cstor pool instance
        poolType:

        # a list of cstor pool instances that can be declared
        # with desired disk count & disk capacity
        items:
        -   # label to uniquely identify a single node in the
            # cluster. In other words this cstor pool instance
            # must get scheduled on this node.
            nodeLabel:
          
            # Desired number of disks that need to participate
            # to build this cstor pool instance
            diskCount:

            # Desired capacity of each disk that participates 
            # in this cstor pool instance
            diskCapacity:
```