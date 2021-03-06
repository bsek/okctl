package reconciler

import (
	"fmt"

	clientCore "github.com/oslokommune/okctl/pkg/client/core"

	"github.com/mishudark/errors"

	"github.com/oslokommune/okctl/pkg/client"
	"github.com/oslokommune/okctl/pkg/controller/resourcetree"
)

// externalDNSReconciler contains service and metadata for the relevant resource
type externalDNSReconciler struct {
	commonMetadata *resourcetree.CommonMetadata
	stateHandlers  *clientCore.StateHandlers

	client client.ExternalDNSService
}

// NodeType returns the relevant ResourceNodeType for this reconciler
func (z *externalDNSReconciler) NodeType() resourcetree.ResourceNodeType {
	return resourcetree.ResourceNodeTypeExternalDNS
}

// SetCommonMetadata saves common metadata for use in Reconcile()
func (z *externalDNSReconciler) SetCommonMetadata(metadata *resourcetree.CommonMetadata) {
	z.commonMetadata = metadata
}

// SetStateHandlers sets the state handlers
func (z *externalDNSReconciler) SetStateHandlers(handlers *clientCore.StateHandlers) {
	z.stateHandlers = handlers
}

// Reconcile knows how to do what is necessary to ensure the desired state is achieved
func (z *externalDNSReconciler) Reconcile(node *resourcetree.ResourceNode) (result ReconcilationResult, err error) {
	switch node.State {
	case resourcetree.ResourceNodeStatePresent:
		hz, err := z.stateHandlers.Domain.GetPrimaryHostedZone()
		if err != nil {
			return result, fmt.Errorf("getting primary hosted zone: %w", err)
		}

		_, err = z.client.CreateExternalDNS(z.commonMetadata.Ctx, client.CreateExternalDNSOpts{
			ID:           z.commonMetadata.ClusterID,
			HostedZoneID: hz.HostedZoneID,
			Domain:       z.commonMetadata.Declaration.ClusterRootDomain,
		})
		if err != nil {
			result.Requeue = errors.IsKind(err, errors.Timeout)

			return result, fmt.Errorf("creating external DNS: %w", err)
		}
	case resourcetree.ResourceNodeStateAbsent:
		err = z.client.DeleteExternalDNS(z.commonMetadata.Ctx, z.commonMetadata.ClusterID)
		if err != nil {
			return result, fmt.Errorf("deleting external DNS: %w", err)
		}
	}

	return result, nil
}

// NewExternalDNSReconciler creates a new reconciler for the ExternalDNS resource
func NewExternalDNSReconciler(client client.ExternalDNSService) Reconciler {
	return &externalDNSReconciler{
		client: client,
	}
}
