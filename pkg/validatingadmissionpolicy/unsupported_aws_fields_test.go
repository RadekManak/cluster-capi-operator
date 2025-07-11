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

package validatingadmissionpolicy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	awsv1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/cluster-api/infrastructure/v1beta2"
	corev1resourcebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/core/v1"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	awsv1 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

func createVAPs(cl client.Client, ctx context.Context, vapPath string) {
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{}
	policyBinding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}

	vapsFile, err := os.Open(vapPath)
	Expect(err).ToNot(HaveOccurred())

	decoder := yaml.NewYAMLOrJSONDecoder(vapsFile, 4096)
	// Order is important, binding then yaml.
	Expect(decoder.Decode(policyBinding)).To(Succeed())
	Expect(decoder.Decode(policy)).To(Succeed())
	Expect(vapsFile.Close()).To(Succeed())

	By("creating the VAP and VAPB")
	Expect(cl.Create(ctx, policy)).To(Succeed())
	Expect(cl.Create(ctx, policyBinding)).To(Succeed())
}

var _ = Describe("Unsupported AWS fields validating admission policy", func() {
	var (
		namespace *corev1.Namespace
		k         komega.Komega
	)

	expectVAPError := func(err error, msg string) {
		var statusErr *apierrors.StatusError
		ExpectWithOffset(1, errors.As(err, &statusErr)).To(BeTrue())
		ExpectWithOffset(1, statusErr.Status().Code).To(Equal(int32(http.StatusUnprocessableEntity)))
		ExpectWithOffset(1, statusErr.Error()).To(ContainSubstring(msg))
	}

	BeforeEach(func() {
		createVAPs(k8sClient, ctx, "../../manifests-vaps/aws/0000_30_cluster-api_10_unsupported_aws_fields_validation.yaml")

		By("creating a namespace for the test")
		namespace = corev1resourcebuilder.Namespace().WithGenerateName("unsupported-aws-fields-").Build()
		namespace.SetLabels(map[string]string{
			"name": "openshift-cluster-api",
		})

		k = komega.New(k8sClient)

		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		checkVAPMachine := awsv1resourcebuilder.AWSMachine().WithName("check-vap-machine").WithNamespace(namespace.Name).Build()
		Expect(k8sClient.Create(ctx, checkVAPMachine)).To(Succeed())

		// Continually try to update the AWSMachine to a forbidden field until the VAP blocks it
		Eventually(k.Update(checkVAPMachine, func() {
			checkVAPMachine.Spec.ImageLookupFormat = "forbidden-format"
		})).Should(MatchError(ContainSubstring("spec.imageLookupFormat is a forbidden field")))

	})

	AfterEach(func() {
		// Cleanup all VAPs
		testutils.CleanupResources(Default, ctx, cfg, k8sClient, "",
			&admissionregistrationv1.ValidatingAdmissionPolicy{},
			&admissionregistrationv1.ValidatingAdmissionPolicyBinding{},
		)

		By("deleting the namespace")
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())

	})

	Context("AWSMachine validation", func() {
		const (
			testImageLookupFormat = "ami-format"
			testImageLookupOrg    = "123456789012"
			testImageLookupBaseOS = "linux"
			testSecretPrefix      = "my-secret"
			testSecurityGroupID   = "sg-123"
			testNetworkInterface  = "eni-12345678"
			testVaultBackend      = "vault"
		)

		type awsMachineTestCase struct {
			name          string
			modifier      func(*awsv1.AWSMachine)
			expectedError string
		}

		awsMachineTestCases := []awsMachineTestCase{
			{
				name: "without forbidden fields",
			},
			{
				name: "with a forbidden field (ami.eksOptimizedLookupType)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.AMI.EKSOptimizedLookupType = ptr.To(awsv1.AmazonLinux)
				},
				expectedError: "spec.ami.eksOptimizedLookupType is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupFormat)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.ImageLookupFormat = testImageLookupFormat
				},
				expectedError: "spec.imageLookupFormat is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupOrg)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.ImageLookupOrg = testImageLookupOrg
				},
				expectedError: "spec.imageLookupOrg is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupBaseOS)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.ImageLookupBaseOS = testImageLookupBaseOS
				},
				expectedError: "spec.imageLookupBaseOS is a forbidden field",
			},
			{
				name: "with a forbidden field (networkInterfaces)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.NetworkInterfaces = []string{testNetworkInterface}
				},
				expectedError: "spec.networkInterfaces is a forbidden field",
			},
			{
				name: "with a forbidden field (uncompressedUserData)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.UncompressedUserData = ptr.To(true)
				},
				expectedError: "spec.uncompressedUserData is a forbidden field",
			},
			{
				name: "with a forbidden field (cloudInit)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.CloudInit = awsv1.CloudInit{SecretCount: 1}
				},
				expectedError: "spec.cloudInit is a forbidden field",
			},
			{
				name: "with a forbidden field (privateDNSName)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.PrivateDNSName = &awsv1.PrivateDNSName{}
				},
				expectedError: "spec.privateDnsName is a forbidden field",
			},
			{
				name: "with a forbidden field (ignition.proxy)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.Ignition = &awsv1.Ignition{Proxy: &awsv1.IgnitionProxy{}}
				},
				expectedError: "spec.ignition.proxy is a forbidden field",
			},
			{
				name: "with a forbidden field (ignition.tls)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.Ignition = &awsv1.Ignition{TLS: &awsv1.IgnitionTLS{}}
				},
				expectedError: "spec.ignition.tls is a forbidden field",
			},
			{
				name: "with a forbidden field (securityGroupOverrides)",
				modifier: func(m *awsv1.AWSMachine) {
					m.Spec.SecurityGroupOverrides = map[awsv1.SecurityGroupRole]string{"bastion": testSecurityGroupID}
				},
				expectedError: "spec.securityGroupOverrides is a forbidden field",
			},
		}

		It("should validate AWSMachine creation with various forbidden fields", func() {
			allFailures := []string{}
			for i, tc := range awsMachineTestCases {
				By(tc.name)

				failures := InterceptGomegaFailures(func() {
					awsMachine := awsv1resourcebuilder.AWSMachine().WithName("test-aws-machine-" + strconv.Itoa(i)).WithNamespace(namespace.Name).Build()

					if tc.modifier != nil {
						tc.modifier(awsMachine)
					}

					err := k8sClient.Create(ctx, awsMachine)

					if tc.expectedError != "" {
						Expect(err).To(HaveOccurred())
						expectVAPError(err, tc.expectedError)
					} else {
						Expect(err).ToNot(HaveOccurred())
					}
				})

				if len(failures) > 0 {
					allFailures = append(allFailures, fmt.Sprintf("Failure in test case %q:\n%s", tc.name, strings.Join(failures, "\n")))
				}
			}

			if len(allFailures) > 0 {
				Fail(strings.Join(allFailures, "\n\n"))
			}
		})
	})

	Context("AWSMachineTemplate validation", func() {
		const (
			testImageLookupFormat = "ami-format"
			testImageLookupOrg    = "123456789012"
			testImageLookupBaseOS = "linux"
			testSecretPrefix      = "my-secret"
			testSecurityGroupID   = "sg-123"
			testNetworkInterface  = "eni-12345678"
			testVaultBackend      = "vault"
		)

		type awsMachineTemplateTestCase struct {
			name          string
			modifier      func(*awsv1.AWSMachineTemplate)
			expectedError string
		}

		awsMachineTemplateTestCases := []awsMachineTemplateTestCase{
			{
				name: "without forbidden fields",
			},
			{
				name: "with a forbidden field (ami.eksOptimizedLookupType)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.AMI.EKSOptimizedLookupType = ptr.To(awsv1.AmazonLinux)
				},
				expectedError: "spec.ami.eksOptimizedLookupType is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupFormat)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.ImageLookupFormat = testImageLookupFormat
				},
				expectedError: "spec.imageLookupFormat is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupOrg)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.ImageLookupOrg = testImageLookupOrg
				},
				expectedError: "spec.imageLookupOrg is a forbidden field",
			},
			{
				name: "with a forbidden field (imageLookupBaseOS)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.ImageLookupBaseOS = testImageLookupBaseOS
				},
				expectedError: "spec.imageLookupBaseOS is a forbidden field",
			},
			{
				name: "with a forbidden field (networkInterfaces)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.NetworkInterfaces = []string{testNetworkInterface}
				},
				expectedError: "spec.networkInterfaces is a forbidden field",
			},
			{
				name: "with a forbidden field (uncompressedUserData)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.UncompressedUserData = ptr.To(true)
				},
				expectedError: "spec.uncompressedUserData is a forbidden field",
			},
			{
				name: "with a forbidden field (cloudInit)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.CloudInit = awsv1.CloudInit{SecretCount: 1}
				},
				expectedError: "spec.cloudInit is a forbidden field",
			},
			{
				name: "with a forbidden field (privateDNSName)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.PrivateDNSName = &awsv1.PrivateDNSName{}
				},
				expectedError: "spec.privateDnsName is a forbidden field",
			},
			{
				name: "with a forbidden field (ignition.proxy)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.Ignition = &awsv1.Ignition{Proxy: &awsv1.IgnitionProxy{}}
				},
				expectedError: "spec.ignition.proxy is a forbidden field",
			},
			{
				name: "with a forbidden field (ignition.tls)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.Ignition = &awsv1.Ignition{TLS: &awsv1.IgnitionTLS{}}
				},
				expectedError: "spec.ignition.tls is a forbidden field",
			},
			{
				name: "with a forbidden field (securityGroupOverrides)",
				modifier: func(mt *awsv1.AWSMachineTemplate) {
					mt.Spec.Template.Spec.SecurityGroupOverrides = map[awsv1.SecurityGroupRole]string{"bastion": testSecurityGroupID}
				},
				expectedError: "spec.securityGroupOverrides is a forbidden field",
			},
		}

		It("should validate AWSMachineTemplate creation with various forbidden fields", func() {
			allFailures := []string{}
			for i, tc := range awsMachineTemplateTestCases {
				By(tc.name)

				failures := InterceptGomegaFailures(func() {
					awsMachineTemplate := awsv1resourcebuilder.AWSMachineTemplate().WithName("test-aws-machine-template-" + strconv.Itoa(i)).WithNamespace(namespace.Name).Build()

					if tc.modifier != nil {
						tc.modifier(awsMachineTemplate)
					}

					err := k8sClient.Create(ctx, awsMachineTemplate)

					if tc.expectedError != "" {
						Expect(err).To(HaveOccurred())
						expectVAPError(err, tc.expectedError)
					} else {
						Expect(err).ToNot(HaveOccurred())
					}
				})

				if len(failures) > 0 {
					allFailures = append(allFailures, fmt.Sprintf("Failure in test case %q:\n%s", tc.name, strings.Join(failures, "\n")))
				}
			}

			if len(allFailures) > 0 {
				Fail(strings.Join(allFailures, "\n\n"))
			}
		})

		It("should prevent updates that add a forbidden field", func() {
			awsMachineTemplate := awsv1resourcebuilder.AWSMachineTemplate().WithName("test-aws-machine-template").WithNamespace(namespace.Name).Build()
			Expect(k8sClient.Create(ctx, awsMachineTemplate)).To(Succeed())

			awsMachineTemplate.Spec.Template.Spec.ImageLookupBaseOS = testImageLookupBaseOS
			err := k8sClient.Update(ctx, awsMachineTemplate)
			Expect(err).To(HaveOccurred())
			expectVAPError(err, "spec.imageLookupBaseOS is a forbidden field")
		})

		It("should allow updates that do not add a forbidden field", func() {
			awsMachineTemplate := awsv1resourcebuilder.AWSMachineTemplate().WithName("test-aws-machine-template").WithNamespace(namespace.Name).Build()
			Expect(k8sClient.Create(ctx, awsMachineTemplate)).To(Succeed())

			awsMachineTemplate.Spec.Template.Spec.InstanceType = "m6.large"
			Expect(k8sClient.Update(ctx, awsMachineTemplate)).To(Succeed())
		})

		It("should not enforce the VAP on other namespaces", func() {
			otherNamespace := corev1resourcebuilder.Namespace().WithName("other-namespace").Build()
			Expect(k8sClient.Create(ctx, otherNamespace)).To(Succeed())

			awsMachineTemplate := awsv1resourcebuilder.AWSMachineTemplate().WithName("test-aws-machine-template").WithNamespace(otherNamespace.Name).Build()
			awsMachineTemplate.Spec.Template.Spec.ImageLookupBaseOS = testImageLookupBaseOS
			err := k8sClient.Create(ctx, awsMachineTemplate)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
