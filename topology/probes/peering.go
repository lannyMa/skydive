/*
 * Copyright (C) 2016 Red Hat, Inc.
 *
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 */

package probes

import (
	"github.com/skydive-project/skydive/logging"
	"github.com/skydive-project/skydive/topology"
	"github.com/skydive-project/skydive/topology/graph"
)

// PeeringProbe describes graph peering based on MAC address and graph events
type PeeringProbe struct {
	graph.DefaultGraphListener
	graph *graph.Graph
	peers map[string]*graph.Node
}

func (p *PeeringProbe) onNodeEvent(n *graph.Node) {
	if mac, _ := n.GetFieldString("MAC"); mac != "" {
		if node, ok := p.peers[mac]; ok {
			if !topology.HaveLayer2Link(p.graph, node, n, nil) {
				topology.AddLayer2Link(p.graph, node, n, nil)
			}
			return
		}
	}
	if mac, _ := n.GetFieldString("PeerIntfMAC"); mac != "" {
		nodes := p.graph.GetNodes(graph.Metadata{"MAC": mac})
		switch len(nodes) {
		case 1:
			if !topology.HaveLayer2Link(p.graph, n, nodes[0], nil) {
				topology.AddLayer2Link(p.graph, n, nodes[0], nil)
			}
			fallthrough
		case 0:
			p.peers[mac] = n
		default:
			logging.GetLogger().Errorf("Multiple peer MAC found: %s", mac)
		}

	}
}

// OnNodeUpdated event
func (p *PeeringProbe) OnNodeUpdated(n *graph.Node) {
	p.onNodeEvent(n)
}

// OnNodeAdded event
func (p *PeeringProbe) OnNodeAdded(n *graph.Node) {
	p.onNodeEvent(n)
}

// OnNodeDeleted event
func (p *PeeringProbe) OnNodeDeleted(n *graph.Node) {
	for mac, node := range p.peers {
		if n.ID == node.ID {
			delete(p.peers, mac)
		}
	}
}

// Start the MAC peering resolver probe
func (p *PeeringProbe) Start() {
}

// Stop the probe
func (p *PeeringProbe) Stop() {
	p.graph.RemoveEventListener(p)
}

// NewPeeringProbe creates a new graph node peering probe
func NewPeeringProbe(g *graph.Graph) *PeeringProbe {
	probe := &PeeringProbe{
		graph: g,
		peers: make(map[string]*graph.Node),
	}
	g.AddEventListener(probe)

	return probe
}
