// Copyright 2022 Authors of spidernet-io
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	calicov1 "github.com/tigera/operator/pkg/apis/crd.projectcalico.org/v1"

	"github.com/spidernet-io/egressgateway/pkg/config"
	egressv1beta1 "github.com/spidernet-io/egressgateway/pkg/k8s/apis/egressgateway.spidernet.io/v1beta1"
	"github.com/spidernet-io/egressgateway/pkg/utils"
)

type eciReconciler struct {
	eci               *egressv1beta1.EgressClusterInfo
	client            client.Client
	log               *zap.Logger
	config            *config.Config
	doOnce            sync.Once
	nodeIPv4Map       map[string]string
	nodeIPv6Map       map[string]string
	calicoV4IPPoolMap map[string]string
	calicoV6IPPoolMap map[string]string
}

const (
	defaultEgressClusterInfoName = "default"
	calico                       = "calico"
	serviceClusterIpRange        = "service-cluster-ip-range"
)

var apiServerPodLabel = map[string]string{"component": "kube-apiserver"}

func (r *eciReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	kind, newReq, err := utils.ParseKindWithReq(req)
	if err != nil {
		r.log.Sugar().Infof("parse req(%v) with error: %v", req, err)
		return reconcile.Result{}, err
	}
	log := r.log.With(
		zap.String("namespacedName", newReq.NamespacedName.String()),
		zap.String("kind", kind),
	)

	r.doOnce.Do(func() {
		r.log.Sugar().Info("first reconcile of egressClusterInfo controller, init egressClusterInfo")
	redo:
		err := r.initEgressClusterInfo()
		if err != nil {
			r.log.Sugar().Errorf("first reconcile of egressClusterInfo controller, init egressClusterInfo, with error: %v", err)
			goto redo
		}
	})

	log.Info("reconciling")
	switch kind {
	case "Node":
		return r.reconcileNode(ctx, newReq, log)
	case "IPPool":
		return r.reconcileCalicoIPPool(ctx, newReq, log)
	default:
		return reconcile.Result{}, nil
	}
}

// reconcileCalicoIPPool reconcile calico IPPool
func (r *eciReconciler) reconcileCalicoIPPool(ctx context.Context, req reconcile.Request, log *zap.Logger) (reconcile.Result, error) {
	// eci
	err := r.getEgressClusterInfo()
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	deleted := false
	ippool := new(calicov1.IPPool)
	err = r.client.Get(ctx, req.NamespacedName, ippool)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Sugar().Errorf("Failed to Get ippool, other err: %v", err)
			return reconcile.Result{Requeue: true}, err
		}
		deleted = true
	}
	deleted = deleted || !ippool.GetDeletionTimestamp().IsZero()

	// delete event
	if deleted {
		log.Sugar().Infof("reconcileCalicoIPPool: Delete %s event", req.Name)
		// check calicoV4IPPoolMap and calicoV6IPPoolMap
		log.Sugar().Debugf("reconcileCalicoIPPool: r.calicoV4IPPoolMap: %v; r.calicoV6IPPoolMap: %v", r.calicoV4IPPoolMap, r.calicoV6IPPoolMap)
		cidr, v4ok := r.calicoV4IPPoolMap[req.Name]
		if v4ok {
			// need to delete cidr from calicoV4IPPoolMap
			log.Sugar().Debugf("reconcileCalicoIPPool: calicoV4IPPoolMap delete %s", req.Name)
			delete(r.calicoV4IPPoolMap, req.Name)
			// update eci status
			cidrs := r.getCalicoV4IPPoolsCidrs()
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv4 = cidrs
			log.Sugar().Debugf("reconcileCalicoIPPool: eci.Status.EgressIgnoreCIDR.PodCIDR.IPv4: %v", cidrs)
			err := r.updateEgressClusterInfo()
			if err != nil {
				log.Sugar().Debugf("Failed to updateEgressClusterInfo, err: %v", err)
				r.calicoV4IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		}

		cidr, v6ok := r.calicoV6IPPoolMap[req.Name]
		if v6ok {
			// need to delete cidr from calicoV6IPPoolMap
			log.Sugar().Debugf("reconcileCalicoIPPool: calicoV6IPPoolMap delete %s", req.Name)
			delete(r.calicoV6IPPoolMap, req.Name)
			// update eci status
			cidrs := r.getCalicoV6IPPoolsCidrs()
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv6 = cidrs
			log.Sugar().Debugf("reconcileCalicoIPPool: eci.Status.EgressIgnoreCIDR.PodCIDR.IPv6: %v", cidrs)
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.calicoV6IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		}
		// need not update calicoIPPoolMap
		return reconcile.Result{}, nil
	}

	// not delete event
	log.Sugar().Infof("reconcileCalicoIPPool: Update %s event", req.Name)

	// check if cidr about ippools changed
	isv4Cidr, err := utils.IsIPv4Cidr(ippool.Spec.CIDR)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}
	isv6Cidr, err := utils.IsIPv6Cidr(ippool.Spec.CIDR)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	cidr, ok := r.calicoV4IPPoolMap[req.Name]
	if ok {
		// v4PoolName but v6Cidr, delete it from calicoV4IPPoolMap
		if isv6Cidr {
			// update calicoV4IPPoolMap
			delete(r.calicoV4IPPoolMap, req.Name)
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv4 = r.getCalicoV4IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.calicoV4IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		} else if ippool.Spec.CIDR != cidr {
			// need to update calicoV4IPPoolMap
			r.calicoV4IPPoolMap[req.Name] = ippool.Spec.CIDR
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv4 = r.getCalicoV4IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.calicoV4IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		}
	} else {
		if isv4Cidr {
			// need to update calicoV4IPPoolMap
			r.calicoV4IPPoolMap[req.Name] = ippool.Spec.CIDR
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv4 = r.getCalicoV4IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				delete(r.calicoV4IPPoolMap, req.Name)
				return reconcile.Result{Requeue: true}, err
			}
		}
	}

	cidr, ok = r.calicoV6IPPoolMap[req.Name]
	if ok {
		// v6PoolName but v4Cidr, delete it from calicoV6IPPoolMap
		if isv4Cidr {
			// update calicoV6IPPoolMap
			delete(r.calicoV6IPPoolMap, req.Name)
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv6 = r.getCalicoV6IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.calicoV6IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		} else if ippool.Spec.CIDR != cidr {
			// need to update calicoV6IPPoolMap
			r.calicoV6IPPoolMap[req.Name] = ippool.Spec.CIDR
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv6 = r.getCalicoV6IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.calicoV6IPPoolMap[req.Name] = cidr
				return reconcile.Result{Requeue: true}, err
			}
		}
	} else {
		if isv6Cidr {
			// need to update calicoV6IPPoolMap
			r.calicoV6IPPoolMap[req.Name] = ippool.Spec.CIDR
			r.eci.Status.EgressIgnoreCIDR.PodCIDR.IPv6 = r.getCalicoV6IPPoolsCidrs()
			err := r.updateEgressClusterInfo()
			if err != nil {
				delete(r.calicoV6IPPoolMap, req.Name)
				return reconcile.Result{Requeue: true}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// reconcileNode reconcile node
func (r *eciReconciler) reconcileNode(ctx context.Context, req reconcile.Request, log *zap.Logger) (reconcile.Result, error) {
	// eci
	err := r.getEgressClusterInfo()
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	deleted := false
	node := new(corev1.Node)
	err = r.client.Get(ctx, req.NamespacedName, node)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{Requeue: true}, err
		}
		deleted = true
	}
	deleted = deleted || !node.GetDeletionTimestamp().IsZero()

	// delete event
	if deleted {
		log.Sugar().Infof("reconcileNode: Delete %s event", req.Name)
		// check map
		nodeipv4, v4Ok := r.nodeIPv4Map[req.Name]
		nodeipv6, v6Ok := r.nodeIPv6Map[req.Name]
		if v4Ok {
			// update map
			delete(r.nodeIPv4Map, req.Name)
			// update eci
			r.eci.Status.EgressIgnoreCIDR.NodeIP.IPv4 = r.getNodesIPv4()
		}
		if v6Ok {
			// update map
			delete(r.nodeIPv6Map, req.Name)
			// update eci
			r.eci.Status.EgressIgnoreCIDR.NodeIP.IPv6 = r.getNodesIPv6()
		}

		// eci need to update
		if v4Ok && v6Ok {
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.nodeIPv4Map[req.Name] = nodeipv4
				r.nodeIPv6Map[req.Name] = nodeipv6
				return reconcile.Result{Requeue: true}, err
			}
		}
		if v4Ok && !v6Ok {
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.nodeIPv4Map[req.Name] = nodeipv4
				return reconcile.Result{Requeue: true}, err
			}
		}
		if !v4Ok && v6Ok {
			err := r.updateEgressClusterInfo()
			if err != nil {
				r.nodeIPv6Map[req.Name] = nodeipv6
				return reconcile.Result{Requeue: true}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// not delete event
	log.Sugar().Infof("reconcileNode: Update %s event", req.Name)

	// get nodeIP, check if its changed
	nodeIPv4, nodeIPv6 := utils.GetNodeIP(node)
	oldNodeIPv4Map := r.nodeIPv4Map
	oldNodeIPv6Map := r.nodeIPv6Map

	_, v4Ok := r.nodeIPv4Map[req.Name]
	_, v6Ok := r.nodeIPv6Map[req.Name]

	needUpdateECI := false
	if (!v4Ok || r.nodeIPv4Map[req.Name] != nodeIPv4) && len(nodeIPv4) != 0 {
		needUpdateECI = true
		// update map
		r.nodeIPv4Map[req.Name] = nodeIPv4

		// need to update node ip from eci status
		r.eci.Status.EgressIgnoreCIDR.NodeIP.IPv4 = r.getNodesIPv4()

	}

	if (!v6Ok || r.nodeIPv6Map[req.Name] != nodeIPv6) && len(nodeIPv6) != 0 {
		needUpdateECI = true
		// update map
		r.nodeIPv6Map[req.Name] = nodeIPv6

		// need to update node ip from eci status
		r.eci.Status.EgressIgnoreCIDR.NodeIP.IPv6 = r.getNodesIPv6()
	}

	if needUpdateECI {
		err = r.updateEgressClusterInfo()
		if err != nil {
			r.nodeIPv4Map = oldNodeIPv4Map
			r.nodeIPv6Map = oldNodeIPv6Map
			return reconcile.Result{Requeue: true}, err
		}
	}

	return reconcile.Result{}, nil
}

func newEgressClusterInfoController(mgr manager.Manager, log *zap.Logger, cfg *config.Config) error {
	if log == nil {
		return fmt.Errorf("log can not be nil")
	}
	if cfg == nil {
		return fmt.Errorf("cfg can not be nil")
	}

	r := &eciReconciler{
		eci:               new(egressv1beta1.EgressClusterInfo),
		client:            mgr.GetClient(),
		log:               log,
		config:            cfg,
		doOnce:            sync.Once{},
		nodeIPv4Map:       make(map[string]string),
		nodeIPv6Map:       make(map[string]string),
		calicoV4IPPoolMap: make(map[string]string),
		calicoV6IPPoolMap: make(map[string]string),
	}

	log.Sugar().Infof("new egressClusterInfo controller")
	c, err := controller.New("egressClusterInfo", mgr,
		controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	podCidr, _, ignoreNodeIP := r.getEgressIgnoreCIDRConfig()

	if ignoreNodeIP {
		log.Sugar().Infof("egressClusterInfo controller watch Node")
		if err := watchSource(c, &source.Kind{Type: &corev1.Node{}}, "Node"); err != nil {
			return err
		}
	}

	switch podCidr {
	case calico:
		log.Sugar().Infof("egressClusterInfo controller watch calico")
		if err := watchSource(c, &source.Kind{Type: &calicov1.IPPool{}}, "IPPool"); err != nil {
			log.Sugar().Errorf("egressClusterInfo controller failed to watch calico")
			return err
		}
	default:
	}

	return nil
}

// initEgressClusterInfo create EgressClusterInfo cr if it's not exists
func (r *eciReconciler) initEgressClusterInfo() error {
	r.log.Sugar().Infof("Start initEgressClusterInfo")
	r.log.Sugar().Infof("Init egressClusterInfo %v", defaultEgressClusterInfoName)
	err := r.getOrCreateEgressClusterInfo()
	if err != nil {
		return err
	}

	_, ignoreClusterCidr, _ := r.getEgressIgnoreCIDRConfig()
	if !ignoreClusterCidr {
		return nil
	}

	// get service-cluster-ip-range from api-server pod
	apiServerPodList := corev1.PodList{}
	opts := client.MatchingLabels(apiServerPodLabel)
	err = r.client.List(context.Background(), &apiServerPodList, opts)
	if err != nil {
		s := fmt.Sprintf("Failed to List api-server pod, err(%v)", err)
		r.log.Sugar().Errorf(s)
		return fmt.Errorf(s)
	}
	apiServerPods := apiServerPodList.Items
	if len(apiServerPods) != 0 {
		apiServerContainers := apiServerPodList.Items[0].Spec.Containers
		if len(apiServerContainers) != 0 {
			commands := apiServerContainers[0].Command
			ipRange := ""
			for _, c := range commands {
				if strings.Contains(c, serviceClusterIpRange) {
					ipRange = strings.Split(c, "=")[1]
					break
				}
			}
			if len(ipRange) == 0 {
				s := fmt.Sprintf("Failed to found service-cluster-ip-range")
				r.log.Sugar().Errorf(s)
				return fmt.Errorf(s)
			}
			// get service-cluster-ip-range, update it to eci status
			ipRanges := strings.Split(ipRange, ",")
			var ipv4Range, ipv6Range string
			if len(ipRanges) == 1 {
				ipv4Range = ipRanges[0]
			}
			if len(ipRanges) == 2 {
				ipv4Range, ipv6Range = ipRanges[0], ipRanges[1]
			}
			r.log.Sugar().Infof("Get service-cluster-ip-range, ipv4Range: %v, ipv6Range: %v", ipv4Range, ipv6Range)
			r.eci.Status.EgressIgnoreCIDR.ClusterIP.IPv4 = []string{ipv4Range}
			r.eci.Status.EgressIgnoreCIDR.ClusterIP.IPv6 = []string{ipv6Range}

		} else {
			s := fmt.Sprintf("Failed to found api-server-pod containers")
			r.log.Sugar().Errorf(s)
			return fmt.Errorf(s)
		}
	} else {
		s := fmt.Sprintf("Failed to found api-server pod")
		r.log.Sugar().Errorf(s)
		return fmt.Errorf(s)
	}
	r.log.Sugar().Debugf("EgressCluterInfo: %v", r.eci)
	r.log.Sugar().Infof("Update EgressClusterInfo: %v", r.eci.Name)
	err = r.updateEgressClusterInfo()
	if err != nil {
		s := fmt.Sprintf("Failed to update EgressClusterInfo, err(%v)", err)
		r.log.Sugar().Errorf(s)
		return fmt.Errorf(s)
	}

	return nil
}

func watchSource(c controller.Controller, source source.Source, kind string) error {
	if err := c.Watch(source, handler.EnqueueRequestsFromMapFunc(utils.KindToMapFlat(kind))); err != nil {
		return fmt.Errorf("failed to watch %s: %w", kind, err)
	}
	return nil
}

// getOrCreateEgressClusterInfo get EgressClusterInfo, if not found create
func (r *eciReconciler) getOrCreateEgressClusterInfo() error {
	err := r.getEgressClusterInfo()
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// not found
		r.eci.Name = defaultEgressClusterInfoName
		r.log.Sugar().Infof("create EgressClusterInfo: %v", r.eci.Name)
		err := r.client.Create(context.Background(), r.eci)
		if err != nil {
			return err
		}
	}
	return nil
}

// getCalicoV4IPPoolsCidrs get calico all ipv4 ippools cidrs from calicoV4IPPoolMap
func (r *eciReconciler) getCalicoV4IPPoolsCidrs() []string {
	cidrs := make([]string, 0)
	for _, cidr := range r.calicoV4IPPoolMap {
		cidrs = append(cidrs, cidr)
	}
	return cidrs
}

// getCalicoV6IPPoolsCidrs get calico all ipv6 ippools cidrs from calicoV6IPPoolMap
func (r *eciReconciler) getCalicoV6IPPoolsCidrs() []string {
	cidrs := make([]string, 0)
	for _, cidr := range r.calicoV6IPPoolMap {
		cidrs = append(cidrs, cidr)
	}
	return cidrs
}

// getNodesIPv4 get all node ipv4 from nodeIPv4Map
func (r *eciReconciler) getNodesIPv4() []string {
	nodeIPs := make([]string, 0)
	for _, v := range r.nodeIPv4Map {
		nodeIPs = append(nodeIPs, v)
	}
	return nodeIPs
}

// getNodesIPv6 get all node ipv6 from nodeIPv6Map
func (r *eciReconciler) getNodesIPv6() []string {
	nodeIPs := make([]string, 0)
	for _, v := range r.nodeIPv6Map {
		nodeIPs = append(nodeIPs, v)
	}
	return nodeIPs
}

// getEgressClusterInfo get EgressClusterInfo cr
func (r *eciReconciler) getEgressClusterInfo() error {
	return r.client.Get(context.Background(), types.NamespacedName{Name: defaultEgressClusterInfoName}, r.eci)
}

// updateEgressClusterInfo update EgressClusterInfo cr
func (r *eciReconciler) updateEgressClusterInfo() error {
	return r.client.Status().Update(context.Background(), r.eci)

}

// getEgressIgnoreCIDRConfig get config about EgressIgnoreCIDR from egressgateway configmap
func (r *eciReconciler) getEgressIgnoreCIDRConfig() (string, bool, bool) {
	i := r.config.FileConfig.EgressIgnoreCIDR
	return i.PodCIDR, i.ClusterIP, i.NodeIP
}
