package main

import (
	"os"

	pocoappv1 "github.com/poconetes/applications/api/v1"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-poco-app", pflag.ExitOnError)
	pflag.CommandLine = flags

	scheme := runtime.NewScheme()
	if err := pocoappv1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	root := NewCmdApp(scheme, genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
