package main

import (
	"bytes"
	"fmt"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/oslokommune/okctl/pkg/api"
	"github.com/oslokommune/okctl/pkg/apis/okctl.io/v1alpha1"
	"github.com/oslokommune/okctl/pkg/config/load"
	"github.com/oslokommune/okctl/pkg/config/state"
	"github.com/oslokommune/okctl/pkg/controller"
	"github.com/oslokommune/okctl/pkg/controller/reconsiler"
	"github.com/oslokommune/okctl/pkg/controller/resourcetree"
	"github.com/oslokommune/okctl/pkg/okctl"
	"github.com/oslokommune/okctl/pkg/spinner"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strconv"
)

type applyClusterOpts struct {
	File string

	Declaration *v1alpha1.Cluster
}

func (o *applyClusterOpts) Validate() error {
	return validation.ValidateStruct(o,
		validation.Field(&o.File, validation.Required),
	)
}

func buildApplyClusterCommand(o *okctl.Okctl) *cobra.Command {
	opts := applyClusterOpts{}

	cmd := &cobra.Command{
		Use: "cluster -f declaration_file",
		Example: "okctl apply cluster -f cluster.yaml",
		Short: "apply a cluster definition to the world",
		Long: "ensures your cluster reflects the declaration of it",
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			opts.Declaration, err = inferClusterFromStdinOrFile(o.In, opts.File)
			if err != nil {
				return fmt.Errorf("error inferring cluster: %w", err)
			}

			err = loadNoUserInputUserData(o, cmd)
			if err != nil {
				return fmt.Errorf("failed to load application data: %w", err)
			}

			err = loadNoUserInputRepoData(o, opts.Declaration)
			if err != nil {
				return fmt.Errorf("failed to load repo data: %w", err)
			}

			err = o.InitialiseWithEnvAndAWSAccountID(
				opts.Declaration.Metadata.Environment,
				strconv.Itoa(opts.Declaration.Metadata.AccountID),
			)
			if err != nil {
				return fmt.Errorf("error initializing okctl: %w", err)
			}
			
			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) (err error) {
			return nil
		},
		
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			id := api.ID{
				Region:       opts.Declaration.Metadata.Region,
				AWSAccountID: strconv.Itoa(opts.Declaration.Metadata.AccountID),
				Environment:  opts.Declaration.Metadata.Environment,
				Repository:   o.RepoStateWithEnv.GetMetadata().Name,
				ClusterName: o.RepoStateWithEnv.GetClusterName(),
			}

			spin, err := spinner.New("synchronizing", o.Err)
			services, err := o.ClientServices(spin)
			if err != nil {
			    return fmt.Errorf("error getting services: %w", err)
			}

			outputDir, _ := o.GetRepoOutputDir(opts.Declaration.Metadata.Environment)

			repoDir, err := o.GetRepoDir()
			if err != nil {
				return fmt.Errorf("could not get Repository dir: %w", err)
			}

			desiredGraph := controller.CreateDesiredStateGraph(opts.Declaration)

			err = controller.ApplyDesiredStateMetadata(desiredGraph, opts.Declaration, repoDir)
			if err != nil {
			    return fmt.Errorf("could not apply desired state metadata: %w", err)
			}

			reconsiliationManager := reconsiler.NewReconsilerManager(&resourcetree.CommonMetadata{
				Ctx: o.Ctx,
				Id:  id,
			})

			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeZone, reconsiler.NewZoneReconsiler(services.Domain))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeVPC, reconsiler.NewVPCReconsiler(services.Vpc))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeCluster, reconsiler.NewClusterReconsiler(services.Cluster))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeExternalSecrets, reconsiler.NewExternalSecretsReconsiler(services.ExternalSecrets))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeALBIngress, reconsiler.NewALBIngressReconsiler(services.ALBIngressController))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeExternalDNS, reconsiler.NewExternalDNSReconsiler(services.ExternalDNS))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeGithub, reconsiler.NewGithubReconsiler(services.Github))
			reconsiliationManager.AddReconsiler(resourcetree.ResourceNodeTypeIdentityManager, reconsiler.NewIdentityManagerReconsiler(services.IdentityManager))
			
			synchronizeOpts := &controller.SynchronizeOpts{
				DesiredTree:           desiredGraph,
				ReconsiliationManager: reconsiliationManager,
				Fs:                    o.FileSystem,
				OutputDir:             outputDir,
				GithubGetter:          o.RepoStateWithEnv.GetGithub,
				GithubSetter:          o.RepoStateWithEnv.SaveGithub,
				CIDRGetter: func() string { return o.RepoStateWithEnv.GetVPC().CIDR },
				PrimaryHostedZoneDomainGetter: func() string { return o.RepoStateWithEnv.GetPrimaryHostedZone().Domain },
				PrimaryHostedZoneIDGetter: func() string { return o.RepoStateWithEnv.GetPrimaryHostedZone().Domain },
				PrimaryHostedZoneGetter: func() *state.HostedZone { return o.RepoStateWithEnv.GetPrimaryHostedZone() },
			}

			err = controller.Synchronize(synchronizeOpts)
			if err != nil {
			    return fmt.Errorf("error synchronizing declaration with state: %w", err)
			}
			
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.File, "file", "f", "", usageApplyClusterFile)
	
	cmd.Hidden = true // TODO: remove when feature is complete

	return cmd
}

const usageApplyClusterFile = `specifies where to read the declaration from. Use "-" for stdin`


func inferClusterFromStdinOrFile(stdin io.Reader, path string) (*v1alpha1.Cluster, error) {
	var (
		inputReader io.Reader
		err         error
	)

	switch path {
	case "-":
		inputReader = stdin
	default:
		inputReader, err = os.Open(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("unable to read file: %w", err)
		}
	}

	var (
		buffer bytes.Buffer
		cluster v1alpha1.Cluster
	)
	
	_, err = io.Copy(&buffer, inputReader)
	if err != nil {
	    return nil, fmt.Errorf("error copying reader data: %w", err)
	}

	err = yaml.Unmarshal(buffer.Bytes(), &cluster)
	if err != nil {
	    return nil, fmt.Errorf("error unmarshalling buffer: %w", err)
	}
	
	return &cluster, nil
}

func loadNoUserInputUserData(o *okctl.Okctl, cmd *cobra.Command) error {
	userDataNotFound := load.CreateOnUserDataNotFoundWithNoInput()

	if o.NoInput {
		userDataNotFound = load.ErrOnUserDataNotFound()
	}

	o.UserDataLoader = load.UserDataFromFlagsEnvConfigDefaults(cmd, userDataNotFound)

	return o.LoadUserData()
}

func loadNoUserInputRepoData(o *okctl.Okctl, declaration *v1alpha1.Cluster) error {
	repoDataNotFound := load.CreateOnRepoDataNotFoundWithNoUserInput(declaration)

	o.RepoDataLoader = load.RepoDataFromConfigFile(repoDataNotFound)

	return o.LoadRepoData()
}
