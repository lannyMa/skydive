/*
 * Copyright (C) 2015 Red Hat, Inc.
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

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/redhat-cip/skydive/analyzer"
	"github.com/redhat-cip/skydive/config"
	"github.com/redhat-cip/skydive/flow/mappings"
	fprobes "github.com/redhat-cip/skydive/flow/probes"
	"github.com/redhat-cip/skydive/ovs"
	"github.com/redhat-cip/skydive/topology"
	"github.com/redhat-cip/skydive/topology/graph"
	tprobes "github.com/redhat-cip/skydive/topology/probes"
)

func usage() {
	fmt.Printf("Usage: %s -conf <config.ini> [-h]\n", os.Args[0])
}

func main() {
	filename := flag.String("conf", "/etc/skydive/skydive.ini",
		"Config file with all the skydive parameter.")
	flag.CommandLine.Usage = usage
	flag.Parse()

	err := config.InitConfig(*filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Println("Skydive Agent starting...")

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	backend, err := graph.NewMemoryBackend()
	if err != nil {
		panic(err)
	}

	g, err := graph.NewGraph(backend)
	if err != nil {
		panic(err)
	}

	root := g.NewNode(graph.Identifier(hostname), graph.Metadatas{"Name": hostname, "Type": "host"})
	// send a first reset event to the analyzers
	g.DelSubGraph(root)

	sflowProbe, err := fprobes.NewSFlowProbeFromConfig(g)
	if err != nil {
		panic(err)
	}

	ovsSFlowProbe := ovsdb.SFlowProbe{
		ID:         "SkydiveSFlowProbe",
		Interface:  "eth0",
		Target:     sflowProbe.GetTarget(),
		HeaderSize: 256,
		Sampling:   1,
		Polling:    0,
	}
	sflowHandler := ovsdb.NewOvsSFlowProbesHandler([]ovsdb.SFlowProbe{ovsSFlowProbe})

	ovsmon := ovsdb.NewOvsMonitorFromConfig()
	ovsmon.AddMonitorHandler(sflowHandler)

	analyzers := config.GetConfig().Section("agent").Key("analyzers").Strings(",")
	// TODO(safchain) HA Connection ???
	analyzerAddr := strings.Split(analyzers[0], ":")[0]
	analyzerPort, err := strconv.Atoi(strings.Split(analyzers[0], ":")[1])
	if err != nil {
		panic(err)
	}

	gfe, err := mappings.NewGraphFlowEnhancer(g)
	if err != nil {
		panic(err)
	}

	pipeline := mappings.NewFlowMappingPipeline([]mappings.FlowEnhancer{gfe})
	sflowProbe.SetMappingPipeline(pipeline)

	gclient := graph.NewAsyncClient(analyzerAddr, analyzerPort, "/ws/graph")
	graph.NewForwarder(gclient, g)
	gclient.Connect()

	// start probes that will update the graph
	ns := tprobes.NewNetNSProbe(g, root)
	ns.Start()

	nl := tprobes.NewNetLinkProbe(g, root)
	nl.Start()

	ovs := tprobes.NewOvsdbProbe(g, root, ovsmon)
	ovs.Start()

	analyzer, err := analyzer.NewClient(analyzerAddr, analyzerPort)
	if err != nil {
		panic(err)
	}

	sflowProbe.SetAnalyzerClient(analyzer)
	go sflowProbe.Start()

	ovsmon.StartMonitoring()

	router := mux.NewRouter().StrictSlash(true)

	server := topology.NewServerFromConfig("agent", g, router)
	server.RegisterStaticEndpoints()
	server.RegisterRpcEndpoints()

	gserver, err := graph.NewServerFromConfig(g, nil, server.Router)
	if err != nil {
		panic(err)
	}

	go gserver.ListenAndServe()

	server.ListenAndServe()
}
