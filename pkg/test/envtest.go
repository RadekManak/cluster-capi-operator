/*
Copyright 2024 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	mapiv1 "github.com/openshift/api/machine/v1"
	mapiv1beta1 "github.com/openshift/api/machine/v1beta1"
	"golang.org/x/tools/go/packages"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	awsv1 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	azurev1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	gcpv1 "sigs.k8s.io/cluster-api-provider-gcp/api/v1beta1"
	openstackv1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	. "github.com/onsi/ginkgo/v2"
	configv1 "github.com/openshift/api/config/v1"
	clusteroperatorv1 "github.com/openshift/api/operator/v1"
)

func init() {
	utilruntime.Must(configv1.Install(scheme.Scheme))
	utilruntime.Must(mapiv1.Install(scheme.Scheme))
	utilruntime.Must(mapiv1beta1.Install(scheme.Scheme))
	utilruntime.Must(clusteroperatorv1.Install(scheme.Scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(awsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(azurev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(gcpv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(openstackv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme.Scheme))
}

// StartEnvTest starts a new test environment and returns a client and config.
func StartEnvTest(testEnv *envtest.Environment) (*rest.Config, client.Client, error) {
	// Get the directory containing the openshift/api package.
	openshiftAPIPath, err := getPackageDir(context.TODO(), "github.com/openshift/api")
	if err != nil {
		return nil, nil, err
	}

	testEnv.CRDs = []*apiextensionsv1.CustomResourceDefinition{
		fakeCoreProviderCRD,
		fakeInfrastructureProviderCRD,
		fakeClusterCRD,
		fakeMachineCRD,
		fakeMachineSetCRD,
		fakeControlPlaneMachineSetCRD,
		fakeAWSClusterCRD,
		fakeAWSMachineTemplateCRD,
		fakeAWSMachineCRD,
		fakeAzureClusterCRD,
		fakeGCPClusterCRD,
		fakeOpenStackClusterCRD,
		fakeOpenStackMachineTemplateCRD,
	}

	crdPaths, err := getCRDPaths(path.Join(openshiftAPIPath, "config", "v1", "zz_generated.crd-manifests"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get CRD paths: %w", err)
	}

	testEnv.CRDDirectoryPaths = []string{
		path.Join(openshiftAPIPath, "operator", "v1", "zz_generated.crd-manifests", "0000_10_config-operator_01_configs.crd.yaml"),
	}
	testEnv.ErrorIfCRDPathMissing = true

	testEnv.CRDInstallOptions = envtest.CRDInstallOptions{
		Paths: append([]string{
			path.Join(openshiftAPIPath, "machine", "v1beta1", "zz_generated.crd-manifests", "0000_10_machine-api_01_machinesets-CustomNoUpgrade.crd.yaml"),
			path.Join(openshiftAPIPath, "machine", "v1beta1", "zz_generated.crd-manifests", "0000_10_machine-api_01_machines-CustomNoUpgrade.crd.yaml"),
		}, crdPaths...),
		ErrorIfPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, err
	}

	if cfg == nil {
		return nil, nil, errors.New("envtest.Environment.Start() returned nil config")
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, nil, err
	}

	return cfg, cl, nil
}

// StopEnvTest stops the test environment.
func StopEnvTest(testEnv *envtest.Environment) error {
	return testEnv.Stop()
}

func getPackageDir(ctx context.Context, pkgName string) (string, error) {
	cfg := &packages.Config{
		Mode:    packages.NeedFiles,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, pkgName)
	if err != nil {
		return "", err
	}

	if len(pkgs) == 0 {
		return "", fmt.Errorf("package %s not found", pkgName)
	}

	if len(pkgs) > 1 {
		return "", fmt.Errorf("multiple packages found for %s", pkgName)
	}

	return pkgs[0].Dir, nil
}

// NewVerboseGinkgoLogger sets up a new logr.Logger that writes to GinkoWriter, and uses the passed verbosity
// Useful for debugging.
func NewVerboseGinkgoLogger(verbosity int) logr.Logger {
	return funcr.New(func(prefix, args string) {
		if prefix == "" {
			fmt.Fprintf(GinkgoWriter, "%s\n", args) //nolint:errcheck
		} else {
			fmt.Fprintf(GinkgoWriter, "%s %s\n", prefix, args) //nolint:errcheck
		}
	}, funcr.Options{Verbosity: verbosity})
}

func getCRDPaths(crdDir string) ([]string, error) {
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		return nil, err
	}

	// Regex to identify variants.
	// Matches filenames ending in -Default.crd.yaml, -TechPreviewNoUpgrade.crd.yaml, etc.
	// Group 1: Base Name
	// Group 2: Variant
	variantRegex := regexp.MustCompile(`^(.*)-(Default|TechPreviewNoUpgrade|CustomNoUpgrade|DevPreviewNoUpgrade)\.crd\.yaml$`)

	crdVariants := make(map[string]map[string]string)
	var finalPaths []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".crd.yaml") {
			continue
		}

		fullPath := filepath.Join(crdDir, entry.Name())
		matches := variantRegex.FindStringSubmatch(entry.Name())

		if matches != nil {
			baseName := matches[1]
			variant := matches[2]
			if crdVariants[baseName] == nil {
				crdVariants[baseName] = make(map[string]string)
			}
			crdVariants[baseName][variant] = fullPath
		} else {
			// No variant detected, include it directly
			finalPaths = append(finalPaths, fullPath)
		}
	}

	// Sort base names to ensure deterministic order
	var baseNames []string
	for baseName := range crdVariants {
		baseNames = append(baseNames, baseName)
	}
	sort.Strings(baseNames)

	// Process variants
	for _, baseName := range baseNames {
		variants := crdVariants[baseName]
		if path, ok := variants["TechPreviewNoUpgrade"]; ok {
			finalPaths = append(finalPaths, path)
		} else if path, ok := variants["Default"]; ok {
			finalPaths = append(finalPaths, path)
		} else {
			// Fallback: if neither TechPreview nor Default exists, but others do.
			// Currently, we will just include one of them to ensure the CRD is installed.
			// Prioritize CustomNoUpgrade if available (consistent with machine-api usage)
			// or just pick any.
			found := false
			if path, ok := variants["CustomNoUpgrade"]; ok {
				finalPaths = append(finalPaths, path)
				found = true
			}
			if !found {
				// Just pick one
				for _, path := range variants {
					finalPaths = append(finalPaths, path)
					break
				}
			}
		}
	}

	return finalPaths, nil
}
