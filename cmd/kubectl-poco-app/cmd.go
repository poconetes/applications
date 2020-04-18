package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	appExample = `
	# view the current app in your KUBECONFIG
	%[1]s ns

	# view all of the apps in use by contexts in your KUBECONFIG
	%[1]s ns --list

	# switch your current-context to one that contains the desired app
	%[1]s ns foo
`

	errNoContext = fmt.Errorf("no context is currently set, use %q to select a new one", "kubectl config use-context <context>")
)

// AppOptions provides information required to update
// the current context on a user's KUBECONFIG
type AppOptions struct {
	streams genericclioptions.IOStreams

	configFlags *genericclioptions.ConfigFlags

	scheme *runtime.Scheme

	client    client.Client
	namespace string
	args      []string
}

// NewCmdApp provides a cobra command wrapping AppOptions
func NewCmdApp(scheme *runtime.Scheme, streams genericclioptions.IOStreams) *cobra.Command {
	o := &AppOptions{
		scheme:      scheme,
		streams:     streams,
		configFlags: genericclioptions.NewConfigFlags(true),
	}

	cmd := &cobra.Command{
		Use:          "poco app deploy",
		Short:        "List Apps on namespace",
		Example:      fmt.Sprintf(appExample, "kubectl"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	//cmd.Flags().BoolVar(&o.listApps, "list", o.listApps, "if true, print the list of all apps in the current KUBECONFIG")
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for updating the current context
func (o *AppOptions) Complete(cmd *cobra.Command, args []string) error {
	o.args = args

	if o.configFlags.Namespace != nil {
		o.namespace = *o.configFlags.Namespace
	}

	cfg, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	mapper, err := o.configFlags.ToRESTMapper()
	if err != nil {
		return err
	}

	o.client, err = client.New(cfg, client.Options{Scheme: o.scheme, Mapper: mapper})
	if err != nil {
		return err
	}

	return nil
}

// Validate ensures that all required arguments and flag values are provided
func (o *AppOptions) Validate() error {
	if len(o.args) > 1 {
		return fmt.Errorf("either one or no arguments are allowed")
	}

	return nil
}

// Run lists all available apps on a user's KUBECONFIG or updates the
// current context based on a provided app.
func (o *AppOptions) Run() error {

	return nil
}
