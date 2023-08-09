// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package egressgateway_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spidernet-io/egressgateway/pkg/constant"
	"github.com/spidernet-io/egressgateway/pkg/k8s/apis/v1beta1"
	"github.com/spidernet-io/egressgateway/test/e2e/common"
	"github.com/spidernet-io/egressgateway/test/e2e/tools"
)

var ErrNotNeed = errors.New("not need this case")
var _ = Describe("Operate egressGateway", Serial, Label("egressGateway"), func() {
	var labels map[string]string
	var gatewayName string
	var (
		badDefaultIPv4, badDefaultIPv6 string
		invalidIPv4, invalidIPv6       string
		singleIpv4Pool, singleIpv6Pool []string
		rangeIpv4Pool, rangeIpv6Pool   []string
		cidrIpv4Pool, cidrIpv6Pool     []string
	)
	var labelSelector *metav1.LabelSelector

	//var notGatewayNodes, gatewayNodes []string

	BeforeEach(func() {
		// single Ippools
		singleIpv4Pool, singleIpv6Pool = make([]string, 0), make([]string, 0)
		// range Ippools
		rangeIpv4Pool, rangeIpv6Pool = make([]string, 0), make([]string, 0)
		// cidr Ippools
		cidrIpv4Pool, cidrIpv6Pool = make([]string, 0), make([]string, 0)

		gatewayName = tools.GenerateRandomName("egw")
		labels = map[string]string{gateway: gatewayName}

		labelSelector = &metav1.LabelSelector{MatchLabels: labels}

		if enableV4 {
			badDefaultIPv4 = "11.10.0.1"
			invalidIPv4 = "invalidIPv4"
			singleIpv4Pool = []string{common.RandomIPV4()}
			rangeIpv4Pool = []string{common.RandomIPPoolV4Range("10", "12")}
			cidrIpv4Pool = []string{common.RandomIPPoolV4Cidr("24")}
		}
		if enableV6 {
			badDefaultIPv6 = "fdde:10::1"
			invalidIPv6 = "invalidIPv6"
			singleIpv6Pool = []string{common.RandomIPV6()}
			rangeIpv6Pool = []string{common.RandomIPPoolV6Range("a", "c")}
			cidrIpv6Pool = []string{common.RandomIPPoolV6Cidr("120")}
		}

		DeferCleanup(func() {
			// delete egressgateway if its exists
			GinkgoWriter.Printf("DeleteEgressGatewayIfExists: %s\n", gatewayName)
			time.Sleep(time.Second)
			Expect(common.DeleteEgressGatewayIfExists(f, gatewayName, time.Second*10)).NotTo(HaveOccurred())
		})
	})

	DescribeTable("Failed to create egressGateway", func(checkCreateEG func() error) {
		Expect(checkCreateEG()).To(HaveOccurred())
	},
		Entry("When `Ippools` is invalid", Label("G00001"), func() error {
			return common.CreateEgressGateway(f, common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: []string{invalidIPv4}, IPv6: []string{invalidIPv6}}, v1beta1.NodeSelector{}))
		}),
		// todo bzsuni
		PEntry("When `NodeSelector` is empty", Label("G00002"), func() error {
			return common.CreateEgressGateway(f, common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: singleIpv4Pool, IPv6: singleIpv6Pool}, v1beta1.NodeSelector{}))
		}),
		Entry("When `defaultEIP` is not in `Ippools`", Label("G00003"), func() error {
			return common.CreateEgressGateway(f,
				common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: singleIpv6Pool, IPv6: singleIpv6Pool, Ipv4DefaultEIP: badDefaultIPv4, Ipv6DefaultEIP: badDefaultIPv6},
					v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: labelSelector}))
		}),
		Entry("When the number of `Ippools.IPv4` is not same with `Ippools.IPv6` in dual cluster", Label("G00004"), func() error {
			if enableV4 && enableV6 {
				return common.CreateEgressGateway(f,
					common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: singleIpv4Pool, IPv6: []string{}},
						v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: labelSelector}))
			}
			return ErrNotNeed
		}),
	)

	DescribeTable("Succeeded to create egressGateway", func(createEG func() error) {
		Expect(createEG()).NotTo(HaveOccurred())
	},
		Entry("when `Ippools` is a single IP", Label("G00006"), func() error {
			GinkgoWriter.Printf("singleIpv4Pool: %v, singleIpv6Pool: %v\n", singleIpv4Pool, singleIpv6Pool)
			return common.CreateEgressGateway(f,
				common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: singleIpv4Pool, IPv6: singleIpv6Pool},
					v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: labelSelector}))
		}),
		Entry("when `Ippools` is a IP range like `a-b`", Label("G00007"), func() error {
			GinkgoWriter.Printf("rangeIpv4Pool: %v, rangeIpv6Pool: %v\n", rangeIpv4Pool, rangeIpv6Pool)
			return common.CreateEgressGateway(f,
				common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: rangeIpv4Pool, IPv6: rangeIpv6Pool},
					v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: labelSelector}))
		}),
		// todo bzsuni
		PEntry("when `Ippools` is a IP cidr", Label("G00008"), func() error {
			GinkgoWriter.Printf("cidrIpv4Pool: %v, cidrIpv6Pool: %v\n", cidrIpv4Pool, cidrIpv6Pool)
			return common.CreateEgressGateway(f,
				common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: cidrIpv4Pool, IPv6: cidrIpv6Pool},
					v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: labelSelector}))
		}),
	)

	Context("Edit egressGateway", Serial, func() {
		var egw *v1beta1.EgressGateway
		var v4DefaultEip, v6DefaultEip string
		var nodeA, nodeB *v1.Node
		var nodeAName, nodeBName string
		var nodeALabel, nodeBLabel, notMatchedLabel *metav1.LabelSelector
		var policyName, clusterPolicyName string
		var emptyEgressIP v1beta1.EgressIP
		var podLabel map[string]string
		var dsPodName string
		var expectGatewayStatus *v1beta1.EgressGatewayStatus
		var expectPolicyStatus *v1beta1.EgressPolicyStatus
		var egpObj *v1beta1.EgressPolicy
		var egcpObj *v1beta1.EgressClusterPolicy
		var ns string

		BeforeEach(func() {
			// node
			nodeA = nodeObjs[0]
			nodeB = nodeObjs[1]
			nodeAName = nodeA.Name
			nodeBName = nodeB.Name
			nodeALabel = &metav1.LabelSelector{MatchLabels: nodeA.Labels}
			nodeBLabel = &metav1.LabelSelector{MatchLabels: nodeB.Labels}
			notMatchedLabel = &metav1.LabelSelector{MatchLabels: map[string]string{"notMatchedLabel": ""}}
			GinkgoWriter.Printf("nodeA: %s, labels: %s\n", nodeAName, common.YamlMarshal(nodeALabel))
			GinkgoWriter.Printf("nodeB: %s, labels: %s\n", nodeBName, common.YamlMarshal(nodeBLabel))

			// policy vars
			ns = common.NSDefault
			policyName = tools.GenerateRandomName("egp")
			clusterPolicyName = tools.GenerateRandomName("egcp")
			egpObj = new(v1beta1.EgressPolicy)
			egcpObj = new(v1beta1.EgressClusterPolicy)

			// pod vars
			dsPodName = tools.GenerateRandomName("ds")
			podLabel = map[string]string{"app": dsPodName}

			// generate egressGateway  yaml
			GinkgoWriter.Println("GenerateEgressGatewayYaml")
			egw = common.GenerateEgressGatewayYaml(gatewayName, v1beta1.Ippools{IPv4: rangeIpv4Pool, IPv6: rangeIpv6Pool}, v1beta1.NodeSelector{Policy: common.AVERAGE_SELECTION, Selector: nodeALabel})

			// create egressGateway
			GinkgoWriter.Printf("CreateEgressGateway: %s\n", gatewayName)
			Expect(common.CreateEgressGateway(f, egw)).NotTo(HaveOccurred())

			// wait `DefaultEip` updated in egressGateway status
			GinkgoWriter.Println("WaitEgressGatewayDefaultEIPUpdated")
			v4DefaultEip, v6DefaultEip, err = common.WaitEgressGatewayDefaultEIPUpdated(f, gatewayName, enableV4, enableV6, time.Second*10)
			Expect(err).NotTo(HaveOccurred())

			// generate egressPolicy yaml
			GinkgoWriter.Println("GenerateEgressPolicyYaml")
			egpObj = common.GenerateEgressPolicyYaml(policyName, gatewayName, ns, emptyEgressIP, podLabel, nil, dst)

			// create egressPolicy
			GinkgoWriter.Printf("create egressPolicy: %s\n", policyName)
			Expect(common.CreateEgressPolicy(f, egpObj)).NotTo(HaveOccurred(), "failed to create egressPolicy")

			// check egressPolicy status
			expectPolicyStatus = &v1beta1.EgressPolicyStatus{
				Eip: v1beta1.Eip{
					Ipv4: v4DefaultEip,
					Ipv6: v6DefaultEip,
				},
				Node: nodeAName,
			}
			Expect(common.CheckEgressPolicyStatus(f, policyName, ns, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			// check egressGatewayStatus
			expectGatewayStatus = &v1beta1.EgressGatewayStatus{
				NodeList: []v1beta1.EgressIPStatus{
					{
						Name: nodeAName,
						Eips: []v1beta1.Eips{
							{IPv4: v4DefaultEip, IPv6: v6DefaultEip, Policies: []v1beta1.Policy{
								{Name: policyName, Namespace: ns},
							}},
						},
						Status: string(v1beta1.EgressTunnelReady),
					},
				},
			}
			Expect(common.CheckEgressGatewayStatus(f, gatewayName, expectGatewayStatus, time.Second*5)).NotTo(HaveOccurred())

			// generate egressClusterPolicy yaml
			GinkgoWriter.Println("GenerateEgressClusterPolicyYaml")
			egcpObj = common.GenerateEgressClusterPolicyYaml(clusterPolicyName, gatewayName, emptyEgressIP, podLabel, nil, dst)

			// create egressClusterPolicy
			GinkgoWriter.Printf("create egressClusterPolicy: %s\n", clusterPolicyName)
			Expect(common.CreateEgressPolicy(f, egcpObj)).NotTo(HaveOccurred(), "failed to create egressClusterPolicy")

			// check egressClusterPolicy status
			expectPolicyStatus = &v1beta1.EgressPolicyStatus{
				Eip: v1beta1.Eip{
					Ipv4: v4DefaultEip,
					Ipv6: v6DefaultEip,
				},
				Node: nodeAName,
			}
			Expect(common.CheckEgressClusterPolicyStatus(f, clusterPolicyName, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			// check egressGatewayStatus
			egcp := v1beta1.Policy{Name: clusterPolicyName}

			expectGatewayStatus.NodeList[0].Eips[0].Policies = append(expectGatewayStatus.NodeList[0].Eips[0].Policies, egcp)

			// todo bzsuni egressGateway Bug
			// Expect(common.CheckEgressGatewayStatus(f, gatewayName, expectGatewayStatus, time.Second*5)).NotTo(HaveOccurred())

			DeferCleanup(func() {
				// delete policy if its exists
				GinkgoWriter.Printf("delete egressPolicy: %s if its exists\n", policyName)
				Expect(common.DeleteEgressPolicyIfExists(f, policyName, ns, egpObj, time.Second*10)).NotTo(HaveOccurred())
				GinkgoWriter.Printf("delete egressClusterPolicy: %s if its exists\n", clusterPolicyName)
				Expect(common.DeleteEgressPolicyIfExists(f, clusterPolicyName, "", egcpObj, time.Second*10)).NotTo(HaveOccurred())

			})
		})

		It("`DefaultEip` will be assigned randomly from `Ippools` when the filed is empty", Label("G00005"), func() {
			if enableV4 {
				GinkgoWriter.Printf("Check DefaultEip %s if within range %v\n", v4DefaultEip, rangeIpv4Pool)
				included, err := common.CheckIPIncluded(constant.IPv4, v4DefaultEip, rangeIpv4Pool)
				Expect(err).NotTo(HaveOccurred())
				Expect(included).To(BeTrue())
			}
			if enableV6 {
				GinkgoWriter.Printf("Check DefaultEip %s if within range %v\n", v6DefaultEip, rangeIpv6Pool)
				included, err := common.CheckIPIncluded(constant.IPv6, v6DefaultEip, rangeIpv6Pool)
				Expect(err).NotTo(HaveOccurred())
				Expect(included).To(BeTrue())
			}
		})

		DescribeTable("Test edit EgressGatewaySpec", func(expectOk bool, updateEG func() error) {
			if expectOk {
				Expect(updateEG()).NotTo(HaveOccurred())
			} else {
				Expect(updateEG()).To(HaveOccurred())
			}
		},
			Entry("Failed when add invalid `IP` to `Ippools`", Label("G00009"), false, func() error {
				if enableV4 {
					egw.Spec.Ippools.IPv4 = append(egw.Spec.Ippools.IPv4, invalidIPv4)
				}
				if enableV6 {
					egw.Spec.Ippools.IPv6 = append(egw.Spec.Ippools.IPv6, invalidIPv6)
				}
				GinkgoWriter.Printf("UpdateEgressGateway: %s\n", egw.Name)
				return common.UpdateEgressGateway(f, egw, time.Second*10)
			}),
			Entry("Succeeded when add valid `IP` to `Ippools`", Label("G00012", "G00013"), true, func() error {
				if enableV4 {
					egw.Spec.Ippools.IPv4 = append(egw.Spec.Ippools.IPv4, singleIpv4Pool...)
				}
				if enableV6 {
					egw.Spec.Ippools.IPv6 = append(egw.Spec.Ippools.IPv6, singleIpv6Pool...)
				}
				GinkgoWriter.Printf("UpdateEgressGateway: %s\n", egw.Name)
				return common.UpdateEgressGateway(f, egw, time.Second*10)
			}),
			Entry("Failed when delete `IP` that being used", Label("G00010"), false, func() error {
				if enableV4 {
					egw.Spec.Ippools.IPv4 = tools.RemoveValueFromSlice(egw.Spec.Ippools.IPv4, v4DefaultEip)
				}
				if enableV6 {
					egw.Spec.Ippools.IPv6 = tools.RemoveValueFromSlice(egw.Spec.Ippools.IPv6, v6DefaultEip)
				}
				GinkgoWriter.Printf("UpdateEgressGateway: %s\n", egw.Name)
				return common.UpdateEgressGateway(f, egw, time.Second*10)
			}),
			Entry("Failed when add different number of ip to `Ippools.IPv4` and `Ippools.IPv6`", Label("G00011"), false, func() error {
				if enableV4 && enableV6 {
					egw.Spec.Ippools.IPv4 = append(egw.Spec.Ippools.IPv4, singleIpv4Pool...)
					GinkgoWriter.Printf("UpdateEgressGateway: %s\n", egw.Name)
					return common.UpdateEgressGateway(f, egw, time.Second*10)
				}
				return ErrNotNeed
			}),
		)

		It("Update egressGatewaySpec.NodeSelector", Label("G00014", "G00015", "G00016"), func() {
			By("Change egressGatewaySpec.NodeSelector form nodeALable to nodeBLable")
			egw.Spec.NodeSelector.Selector = nodeBLabel
			Expect(common.UpdateEgressGateway(f, egw, time.Second*10)).NotTo(HaveOccurred())

			// check egressGatewayStatus
			expectGatewayStatus.NodeList[0].Name = nodeBName
			// GinkgoWriter.Printf("We expect gateway: %s update succussfully\n", gatewayName)
			// todo bzsuni egressGateway Bug
			// Expect(common.CheckEgressGatewayStatus(f, gatewayName, expectGatewayStatus, time.Second*5)).NotTo(HaveOccurred())

			// check expectPolicyStatus
			expectPolicyStatus.Node = nodeBName
			GinkgoWriter.Printf("We expect clusterPolicy: %s update succussfully\n", clusterPolicyName)
			Expect(common.CheckEgressClusterPolicyStatus(f, clusterPolicyName, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			GinkgoWriter.Printf("We expect policy: %s update succussfully\n", policyName)
			Expect(common.CheckEgressPolicyStatus(f, policyName, ns, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			// todo check data level

			By("Change egressGatewaySpec.NodeSelector to not match any nodes")
			egw.Spec.NodeSelector.Selector = notMatchedLabel
			Expect(common.UpdateEgressGateway(f, egw, time.Second*10)).NotTo(HaveOccurred())

			// check egressGatewayStatus
			emptyGatewayStatus := &v1beta1.EgressGatewayStatus{}
			GinkgoWriter.Println("We expect the EgressGatewayStatus is emtpty")
			Expect(common.CheckEgressGatewayStatus(f, gatewayName, emptyGatewayStatus, time.Second*5)).NotTo(HaveOccurred())

			// check expectPolicyStatus
			emptyPolicyStatus := &v1beta1.EgressPolicyStatus{}
			GinkgoWriter.Printf("We expect clusterPolicy: %s update succussfully\n", clusterPolicyName)
			Expect(common.CheckEgressClusterPolicyStatus(f, clusterPolicyName, emptyPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			GinkgoWriter.Printf("We expect policy: %s update succussfully\n", policyName)
			Expect(common.CheckEgressPolicyStatus(f, policyName, ns, emptyPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			// todo check data level

			By("Change egressGatewaySpec.NodeSelector form notMatchedLabel to nodeBLable")
			egw.Spec.NodeSelector.Selector = nodeBLabel
			Expect(common.UpdateEgressGateway(f, egw, time.Second*10)).NotTo(HaveOccurred())

			GinkgoWriter.Printf("We expect the nodeName is %s after update\n", nodeBName)

			// check egressGatewayStatus
			// todo bzsuni egressGateway Bug
			// Expect(common.CheckEgressGatewayStatus(f, gatewayName, expectGatewayStatus, time.Second*5)).NotTo(HaveOccurred())

			// check expectPolicyStatus
			GinkgoWriter.Printf("We expect clusterPolicy: %s update succussfully\n", clusterPolicyName)
			Expect(common.CheckEgressClusterPolicyStatus(f, clusterPolicyName, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			GinkgoWriter.Printf("We expect policy: %s update succussfully\n", policyName)
			Expect(common.CheckEgressPolicyStatus(f, policyName, ns, expectPolicyStatus, time.Second*5)).NotTo(HaveOccurred())

			// todo check data level
		})
	})
})
