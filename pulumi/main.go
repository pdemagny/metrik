package main

import (
	u "pulumi/internal/utils"
	"pulumi/pkg/helm/release"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
)

type Release struct {
	Name                string
	Namespace           string
	ChartPath           string
	ChartValuesFilePath string
	ChartVersion        string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		prefix := ctx.Stack() + "-"

		globalConfig := config.New(ctx, "global")
		debug := globalConfig.RequireBool("debug")
		enableComponentResources := globalConfig.RequireBool("enableComponentResources")

		kubernetesConfig := config.New(ctx, "kubernetes")
		kubernetesContext := kubernetesConfig.Require("context")

		var releases []Release
		metrikConfig := config.New(ctx, "metrik")
		err := metrikConfig.TryObject("releases", &releases)
		if err != nil {
			return err
		}

		if len(releases) > 0 {
			kubernetesProvider, err := kubernetes.NewProvider(ctx, prefix+"kubernetes-provider", &kubernetes.ProviderArgs{
				Context: u.Ps(kubernetesContext),
			})
			if err != nil {
				return err
			}

			for _, rel := range releases {
				ns, err := corev1.NewNamespace(ctx, prefix+rel.Namespace, &corev1.NamespaceArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Name: u.Ps(prefix + rel.Namespace),
						Labels: u.Psm(map[string]string{
							"pod-security.kubernetes.io/enforce":         "restricted",
							"pod-security.kubernetes.io/enforce-version": "v1.35",
						}),
					},
				}, pulumi.Provider(kubernetesProvider))
				if err != nil {
					return err
				}

				_, err = release.NewRelease(
					release.WithContext(ctx),
					release.WithResourceOptions([]pulumi.ResourceOption{
						pulumi.Provider(kubernetesProvider),
					}),
					release.WithEnableComponentResource(enableComponentResources),
					release.WithDebug(debug),
					release.WithName(prefix+rel.Name),
					release.WithNamespace(ns.Metadata.Name().Elem()),
					release.WithChart(rel.ChartPath),
					release.WithVersion(rel.ChartVersion),
					release.WithValueYamlFiles(pulumi.AssetOrArchiveArray{
						pulumi.NewFileAsset(rel.ChartValuesFilePath),
					}),
				).Create()
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}
