package controller

import (
	"fmt"
	"github.com/oslokommune/okctl/pkg/controller/reconsiler"
	"github.com/oslokommune/okctl/pkg/controller/resourcetree"
	"github.com/spf13/afero"
)

type SynchronizeOpts struct {
	DesiredTree *resourcetree.ResourceNode
	
	ReconsiliationManager *reconsiler.ReconsilerManager
	
	Fs *afero.Afero
	OutputDir string

	GithubGetter reconsiler.GithubGetter
	GithubSetter reconsiler.GithubSetter

	CIDRGetter StringFetcher
	PrimaryHostedZoneDomainGetter StringFetcher
	PrimaryHostedZoneIDGetter StringFetcher
	PrimaryHostedZoneGetter HostedZoneFetcher
}

// Synchronize knows how to discover differences between desired and actual state and rectify them
func Synchronize(opts *SynchronizeOpts) error {
	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeCluster, CreateClusterStateRefresher(
		opts.Fs,
		opts.OutputDir,
		opts.CIDRGetter,
	))
	
	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeALBIngress, CreateALBIngressControllerRefresher(
		opts.Fs,
		opts.OutputDir,
	))

	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeExternalDNS, CreateExternalDNSStateRefresher(
		opts.PrimaryHostedZoneDomainGetter,
		opts.PrimaryHostedZoneIDGetter,
	))
	
	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeIdentityManager, CreateIdentityManagerRefresher(
		opts.PrimaryHostedZoneDomainGetter,
		opts.PrimaryHostedZoneIDGetter,
	))

	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeGithub, CreateGithubStateRefresher(
		opts.GithubGetter,
		opts.GithubSetter,
	))
	
	opts.DesiredTree.SetStateRefresher(resourcetree.ResourceNodeTypeArgoCD, CreateArgocdStateRefresher(
		opts.PrimaryHostedZoneGetter,
	))

	currentStateGraphOpts, err := NewCreateCurrentStateGraphOpts(opts.Fs, opts.OutputDir)
	if err != nil {
	    return fmt.Errorf("unable to get existing services: %w", err)
	}

	currentStateTree := CreateCurrentStateGraph(currentStateGraphOpts)

	diffGraph := *opts.DesiredTree

	diffGraph.ApplyFunction(applyCurrentState, currentStateTree)

	return handleNode(opts.ReconsiliationManager, &diffGraph)
}

// handleNode knows how to run Reconsile() on every node of a ResourceNode tree
func handleNode(reconsilerManager *reconsiler.ReconsilerManager, currentNode *resourcetree.ResourceNode) error {
	_, err := reconsilerManager.Reconsile(currentNode)
	if err != nil {
	    return fmt.Errorf("error reconsiling node: %w", err)
	}

	for _, node := range currentNode.Children {
		err = handleNode(reconsilerManager, node)
		if err != nil {
		    return fmt.Errorf("error handling node: %w", err)
		}
	}
	
	return nil
}

// applyCurrentState knows how to apply the current state on a desired state ResourceNode tree to produce a diff that
// knows which resources to create, and which resources is already existing
func applyCurrentState(receiver *resourcetree.ResourceNode, target *resourcetree.ResourceNode) {
	if receiver.State == target.State {
		receiver.State = resourcetree.ResourceNodeStateNoop
	}
}

