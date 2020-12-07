package reconsiler

import (
	"github.com/oslokommune/okctl/pkg/controller/resourcetree"
)

/*
 * Reconsiler
 */

// ReconsilationResult contains information about the result of a Reconsile() call
type ReconsilationResult struct {
	// Requeue indicates if this Reconciliation must be run again
	Requeue bool
}

type Reconsiler interface {
	// Reconsile knows how to do what is necessary to ensure the desired state is achieved
	Reconsile(*resourcetree.ResourceNode) (*ReconsilationResult, error)
	SetCommonMetadata(metadata *resourcetree.CommonMetadata)
}

/*
ReconsilerManager provides a simpler way to organize reconsilers
*/
type ReconsilerManager struct {
	commonMetadata *resourcetree.CommonMetadata
	Reconsilers    map[resourcetree.ResourceNodeType]Reconsiler
}

// AddReconsiler makes a Reconsiler available in the ReconsilerManager
func (manager *ReconsilerManager) AddReconsiler(key resourcetree.ResourceNodeType, reconsiler Reconsiler) {
	reconsiler.SetCommonMetadata(manager.commonMetadata)

	manager.Reconsilers[key] = reconsiler
}

// Reconsile chooses the correct reconsiler to use based on a nodes type
func (manager *ReconsilerManager) Reconsile(node *resourcetree.ResourceNode) (*ReconsilationResult, error) {
	node.RefreshState()

	return manager.Reconsilers[node.Type].Reconsile(node)
}

// NewReconsilerManager creates a new ReconsilerManager with a NoopReconsiler already installed
func NewReconsilerManager(metadata *resourcetree.CommonMetadata) *ReconsilerManager {
	return &ReconsilerManager{
		commonMetadata: metadata,
		Reconsilers: map[resourcetree.ResourceNodeType]Reconsiler{
			resourcetree.ResourceNodeTypeGroup: &NoopReconsiler{},
		},
	}
}
