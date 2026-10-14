package main

import (
	"errors"

	"pulumi/internal/stack"
	u "pulumi/internal/utils"
	"pulumi/pkg/helm/release"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
)

// releaseTimeoutSeconds bounds how long a release waits for its pods to become ready, so an
// image that cannot be pulled fails the deployment within 2 minutes.
const releaseTimeoutSeconds = 120

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		prefix := ctx.Stack() + "-"

		globalConfig := config.New(ctx, "global")
		debug := globalConfig.RequireBool("debug")
		enableComponentResources := globalConfig.RequireBool("enableComponentResources")

		kubernetesConfig := config.New(ctx, "kubernetes")
		kubernetesContext := kubernetesConfig.Require("context")

		var releases []stack.Release
		metrikConfig := config.New(ctx, "metrik")
		err := metrikConfig.TryObject("releases", &releases)
		if err != nil {
			return err
		}

		// TrySecret only fails when the key is unset, which tells whether a password is
		// configured without reading the secret as plain text.
		password, passwordErr := metrikConfig.TrySecret("registryPassword")
		registry := stack.Registry{
			Server:          metrikConfig.Get("registryServer"),
			Username:        metrikConfig.Get("registryUsername"),
			PasswordSet:     passwordErr == nil,
			ImagePullSecret: metrikConfig.Get("imagePullSecret"),
		}
		if err := stack.Validate(releases, registry); err != nil {
			return err
		}

		if len(releases) > 0 {
			kubernetesProvider, err := kubernetes.NewProvider(ctx, prefix+"kubernetes-provider", &kubernetes.ProviderArgs{
				Context: u.Ps(kubernetesContext),
			})
			if err != nil {
				return err
			}

			var dockerConfig pulumi.StringOutput
			if registry.Mode() == stack.ModeCreated {
				var ok bool
				dockerConfig, ok = password.ApplyT(func(password string) (string, error) {
					return stack.DockerConfigJSON(registry.Server, registry.Username, password)
				}).(pulumi.StringOutput)
				if !ok {
					return errors.New("registry pull secret: unexpected output type")
				}
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

				resourceOptions := []pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}
				pullSecret := registry.ImagePullSecret
				if registry.Mode() == stack.ModeCreated {
					secret, err := corev1.NewSecret(ctx, prefix+rel.Namespace+"-"+stack.CreatedPullSecretName, &corev1.SecretArgs{
						Metadata: &metav1.ObjectMetaArgs{
							Name:      u.Ps(stack.CreatedPullSecretName),
							Namespace: ns.Metadata.Name(),
						},
						Type:       u.Ps("kubernetes.io/dockerconfigjson"),
						StringData: pulumi.StringMap{".dockerconfigjson": dockerConfig},
					}, pulumi.Provider(kubernetesProvider))
					if err != nil {
						return err
					}
					pullSecret = stack.CreatedPullSecretName
					resourceOptions = append(resourceOptions, pulumi.DependsOn([]pulumi.Resource{secret}))
				}

				_, err = release.NewRelease(
					release.WithContext(ctx),
					release.WithResourceOptions(resourceOptions),
					release.WithEnableComponentResource(enableComponentResources),
					release.WithDebug(debug),
					release.WithName(prefix+rel.Name),
					release.WithNamespace(ns.Metadata.Name().Elem()),
					release.WithChart(rel.ChartPath),
					release.WithVersion(rel.ChartVersion),
					release.WithValueYamlFiles(pulumi.AssetOrArchiveArray{
						pulumi.NewFileAsset(rel.ChartValuesFilePath),
					}),
					release.WithValues(pulumi.ToMap(stack.Values(rel.Image, pullSecret))),
					release.WithTimeout(releaseTimeoutSeconds),
				).Create()
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}
