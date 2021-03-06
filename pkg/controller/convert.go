package controller

import (
	"github.com/oslokommune/okctl/pkg/apis/okctl.io/v1alpha1"

	"github.com/oslokommune/okctl/pkg/helm/charts/externalsecrets"

	"github.com/oslokommune/okctl/pkg/helm/charts/autoscaler"
	"github.com/oslokommune/okctl/pkg/helm/charts/awslbc"
	"github.com/oslokommune/okctl/pkg/helm/charts/blockstorage"
	"github.com/oslokommune/okctl/pkg/helm/charts/kubepromstack"
	lokipkg "github.com/oslokommune/okctl/pkg/helm/charts/loki"
	"github.com/oslokommune/okctl/pkg/helm/charts/promtail"
	"github.com/oslokommune/okctl/pkg/helm/charts/tempo"

	"github.com/asdine/storm/v3"
	"github.com/oslokommune/okctl/pkg/api"
	"github.com/oslokommune/okctl/pkg/cfn"
	"github.com/pkg/errors"

	clientCore "github.com/oslokommune/okctl/pkg/client/core"

	"github.com/oslokommune/okctl/pkg/controller/resourcetree"
)

// ExistingResources contains information about what services already exists in a cluster
type ExistingResources struct {
	hasServiceQuotaCheck                  bool
	hasAWSLoadBalancerController          bool
	hasCluster                            bool
	hasExternalDNS                        bool
	hasExternalSecrets                    bool
	hasAutoscaler                         bool
	hasBlockstorage                       bool
	hasKubePromStack                      bool
	hasLoki                               bool
	hasPromtail                           bool
	hasTempo                              bool
	hasIdentityManager                    bool
	hasArgoCD                             bool
	hasPrimaryHostedZone                  bool
	hasVPC                                bool
	hasDelegatedHostedZoneNameservers     bool
	hasDelegatedHostedZoneNameserversTest bool
	hasUsers                              bool
	hasPostgres                           map[string]*v1alpha1.ClusterDatabasesPostgres
}

func isNotFound(_ interface{}, err error) bool {
	return errors.Is(err, storm.ErrNotFound)
}

// IdentifyResourcePresence creates an initialized ExistingResources struct
func IdentifyResourcePresence(id api.ID, handlers *clientCore.StateHandlers) (ExistingResources, error) {
	hz, err := handlers.Domain.GetPrimaryHostedZone()
	if err != nil && !errors.Is(err, storm.ErrNotFound) {
		return ExistingResources{}, err
	}

	dbs, err := handlers.Component.GetPostgresDatabases()
	if err != nil {
		return ExistingResources{}, nil
	}

	haveDBs := map[string]*v1alpha1.ClusterDatabasesPostgres{}

	for _, db := range dbs {
		haveDBs[db.ApplicationName] = &v1alpha1.ClusterDatabasesPostgres{
			Name:      db.ApplicationName,
			User:      db.UserName,
			Namespace: db.Namespace,
		}
	}

	return ExistingResources{
		hasServiceQuotaCheck:                  false,
		hasAWSLoadBalancerController:          !isNotFound(handlers.Helm.GetHelmRelease(awslbc.ReleaseName)),
		hasCluster:                            !isNotFound(handlers.Cluster.GetCluster(id.ClusterName)),
		hasExternalDNS:                        !isNotFound(handlers.ExternalDNS.GetExternalDNS()),
		hasExternalSecrets:                    !isNotFound(handlers.Helm.GetHelmRelease(externalsecrets.ReleaseName)),
		hasAutoscaler:                         !isNotFound(handlers.Helm.GetHelmRelease(autoscaler.ReleaseName)),
		hasBlockstorage:                       !isNotFound(handlers.Helm.GetHelmRelease(blockstorage.ReleaseName)),
		hasKubePromStack:                      !isNotFound(handlers.Helm.GetHelmRelease(kubepromstack.ReleaseName)),
		hasLoki:                               !isNotFound(handlers.Helm.GetHelmRelease(lokipkg.ReleaseName)),
		hasPromtail:                           !isNotFound(handlers.Helm.GetHelmRelease(promtail.ReleaseName)),
		hasTempo:                              !isNotFound(handlers.Helm.GetHelmRelease(tempo.ReleaseName)),
		hasIdentityManager:                    !isNotFound(handlers.IdentityManager.GetIdentityPool(cfn.NewStackNamer().IdentityPool(id.ClusterName))),
		hasArgoCD:                             !isNotFound(handlers.ArgoCD.GetArgoCD()),
		hasPrimaryHostedZone:                  !isNotFound(handlers.Domain.GetPrimaryHostedZone()),
		hasVPC:                                !isNotFound(handlers.Vpc.GetVpc(cfn.NewStackNamer().Vpc(id.ClusterName))),
		hasDelegatedHostedZoneNameservers:     hz != nil && hz.IsDelegated,
		hasDelegatedHostedZoneNameserversTest: false,
		hasUsers:                              false, // For now we will always check if there are missing users
		hasPostgres:                           haveDBs,
	}, nil
}

// CreateResourceDependencyTree creates a tree
func CreateResourceDependencyTree() (root *resourcetree.ResourceNode) {
	root = createNode(nil, resourcetree.ResourceNodeTypeServiceQuota)

	var vpcNode,
		clusterNode,
		primaryHostedZoneNode *resourcetree.ResourceNode

	primaryHostedZoneNode = createNode(root, resourcetree.ResourceNodeTypeZone)
	createNode(primaryHostedZoneNode, resourcetree.ResourceNodeTypeNameserverDelegator)

	vpcNode = createNode(primaryHostedZoneNode, resourcetree.ResourceNodeTypeVPC)
	createNode(vpcNode, resourcetree.ResourceNodeTypeCleanupSG)

	clusterNode = createNode(vpcNode, resourcetree.ResourceNodeTypeCluster)
	createNode(clusterNode, resourcetree.ResourceNodeTypeCleanupALB)
	createNode(clusterNode, resourcetree.ResourceNodeTypeExternalSecrets)
	createNode(clusterNode, resourcetree.ResourceNodeTypeAutoscaler)
	createNode(clusterNode, resourcetree.ResourceNodeTypeBlockstorage)
	createNode(clusterNode, resourcetree.ResourceNodeTypeAWSLoadBalancerController)
	createNode(clusterNode, resourcetree.ResourceNodeTypeExternalDNS)
	createNode(clusterNode, resourcetree.ResourceNodeTypePostgres)

	// All resources that requires SSL / a certificate needs the delegatedNameserversConfirmedNode as a dependency
	delegatedNameserversConfirmedNode := createNode(clusterNode, resourcetree.ResourceNodeTypeNameserversDelegatedTest)

	identityProviderNode := createNode(delegatedNameserversConfirmedNode, resourcetree.ResourceNodeTypeIdentityManager)
	createNode(identityProviderNode, resourcetree.ResourceNodeTypeArgoCD)
	createNode(identityProviderNode, resourcetree.ResourceNodeTypeUsers)
	kubePromStack := createNode(identityProviderNode, resourcetree.ResourceNodeTypeKubePromStack)
	// This is not strictly required, but to a large extent it doesn't make much sense to setup Loki before
	// we have setup grafana.
	loki := createNode(kubePromStack, resourcetree.ResourceNodeTypeLoki)
	createNode(kubePromStack, resourcetree.ResourceNodeTypeTempo)
	// Similarly, it doesn't make sense to install promtail without loki
	createNode(loki, resourcetree.ResourceNodeTypePromtail)

	return root
}

// CreateApplicationResourceDependencyTree creates a dependency tree for applications
func CreateApplicationResourceDependencyTree() (root *resourcetree.ResourceNode) {
	root = createNode(nil, resourcetree.ResourceNodeTypeGroup)

	containerRepositoryNode := createNode(root, resourcetree.ResourceNodeTypeContainerRepository)
	createNode(containerRepositoryNode, resourcetree.ResourceNodeTypeApplication)

	return root
}

func createNode(parent *resourcetree.ResourceNode, nodeType resourcetree.ResourceNodeType) (child *resourcetree.ResourceNode) {
	child = &resourcetree.ResourceNode{
		Type:     nodeType,
		Children: make([]*resourcetree.ResourceNode, 0),
	}

	child.State = resourcetree.ResourceNodeStatePresent

	if parent != nil {
		parent.Children = append(parent.Children, child)
	}

	return child
}
