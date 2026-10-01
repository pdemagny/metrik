package release

import (
	"fmt"

	u "pulumi/internal/utils"

	"github.com/kr/pretty"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	component struct {
		pulumi.ResourceState
		Release *helm.Release
	}
	resourceArgs struct {
		Name           string
		Namespace      pulumi.StringOutput
		Chart          string
		Version        string
		ValueYamlFiles pulumi.AssetOrArchiveArray
	}
	options func(*args)
	args    struct {
		Context                 *pulumi.Context
		ResourceOptions         []pulumi.ResourceOption
		Debug                   bool
		EnableComponentResource bool
		resourceArgs            *resourceArgs
	}
)

func WithContext(ctx *pulumi.Context) options {
	return func(a *args) { a.Context = ctx }
}

func WithResourceOptions(resourceOptions []pulumi.ResourceOption) options {
	return func(a *args) { a.ResourceOptions = resourceOptions }
}

func WithDebug(debug bool) options {
	return func(a *args) { a.Debug = debug }
}

func WithEnableComponentResource(enableComponentResource bool) options {
	return func(a *args) { a.EnableComponentResource = enableComponentResource }
}

func WithName(name string) options {
	return func(a *args) { a.resourceArgs.Name = name }
}

func WithNamespace(namespace pulumi.StringOutput) options {
	return func(a *args) { a.resourceArgs.Namespace = namespace }
}

func WithChart(chart string) options {
	return func(a *args) { a.resourceArgs.Chart = chart }
}

func WithVersion(version string) options {
	return func(a *args) { a.resourceArgs.Version = version }
}

func WithValueYamlFiles(valueYamlFiles pulumi.AssetOrArchiveArray) options {
	return func(a *args) { a.resourceArgs.ValueYamlFiles = valueYamlFiles }
}

func NewRelease(opts ...options) *args {
	// Default values
	obj := args{
		Debug:                   false,
		EnableComponentResource: false,
		resourceArgs:            &resourceArgs{},
	}
	for _, opt := range opts {
		opt(&obj)
	}
	return &obj
}

func (a *args) Create() (*component, error) {
	if a.Debug {
		err := a.Context.Log.Info("helm/release.NewRelease: "+pretty.Sprint(a.resourceArgs), nil)
		if err != nil {
			return nil, err
		}
	}

	// Init the Component Resource
	component := &component{}
	componentResourceName := "metrik:helm:Release"
	releaseResourceType := "kubernetes:helm.sh/v3:Release"
	if a.EnableComponentResource {
		err := a.Context.RegisterComponentResource(componentResourceName, a.resourceArgs.Name+"-cr", component, a.ResourceOptions...)
		if err != nil {
			return nil, err
		}
		a.ResourceOptions = append(a.ResourceOptions, pulumi.Parent(component))
	} else {
		releaseResourceType = componentResourceName + "$" + releaseResourceType
	}

	// Create & attach resources to the CR
	releaseArgs := &helm.ReleaseArgs{
		Name:            u.Ps(a.resourceArgs.Name),
		Namespace:       a.resourceArgs.Namespace,
		CreateNamespace: u.Pb(false),
		Chart:           u.Ps(a.resourceArgs.Chart),
		Version:         u.Ps(a.resourceArgs.Version),
		ValueYamlFiles:  a.resourceArgs.ValueYamlFiles,
	}
	a.ResourceOptions = append(a.ResourceOptions, pulumi.Aliases([]pulumi.Alias{{URN: pulumi.URN(fmt.Sprintf("urn:pulumi:%s::%s::%s::%s", a.Context.Stack(), a.Context.Project(), releaseResourceType, a.resourceArgs.Name))}}))
	release, err := helm.NewRelease(a.Context, a.resourceArgs.Name, releaseArgs, a.ResourceOptions...)
	if err != nil {
		return nil, err
	}
	component.Release = release

	// Register CR Outputs
	if a.EnableComponentResource {
		err := a.Context.RegisterResourceOutputs(component, pulumi.Map{})
		if err != nil {
			return nil, err
		}
	}

	return component, nil
}
