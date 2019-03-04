/*
Copyright 2018 The Knative Authors

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

package dag

import (
	"fmt"
	"strings"

	"github.com/knative/build-pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/knative/build-pipeline/pkg/list"
)

// Node represents a Task in a pipeline.
type Node struct {
	// Task represent the PipelineTask in Pipeline
	Task v1alpha1.PipelineTask
	// Prev represent all the Previous task Nodes for the current Task
	Prev []*Node
	// Next represent all the Next task Nodes for the current Task
	Next []*Node
}

// DAG represents the Pipeline DAG
type DAG struct {
	//Nodes represent map of PipelineTask name to Node in Pipeline DAG
	Nodes map[string]*Node
}

// Returns an empty Pipeline DAG
func new() *DAG {
	return &DAG{Nodes: map[string]*Node{}}
}

func (g *DAG) addPipelineTask(t v1alpha1.PipelineTask) (*Node, error) {
	if _, ok := g.Nodes[t.Name]; ok {
		return nil, fmt.Errorf("duplicate pipeline taks")
	}
	newNode := &Node{
		Task: t,
	}
	g.Nodes[t.Name] = newNode
	return newNode, nil
}

func linkPipelineTasks(prev *Node, next *Node) error {
	// Check for self cycle
	if prev.Task.Name == next.Task.Name {
		return fmt.Errorf("cycle detected; task %q depends on itself", next.Task.Name)
	}
	// Check if we are adding cycles.
	visited := map[string]bool{prev.Task.Name: true, next.Task.Name: true}
	path := []string{next.Task.Name, prev.Task.Name}
	if err := visit(next.Task.Name, prev.Prev, path, visited); err != nil {
		return fmt.Errorf("cycle detected: %v", err)
	}
	next.Prev = append(next.Prev, prev)
	prev.Next = append(prev.Next, next)
	return nil
}

func visit(currentName string, nodes []*Node, path []string, visited map[string]bool) error {
	for _, n := range nodes {
		path = append(path, n.Task.Name)
		if _, ok := visited[n.Task.Name]; ok {
			return fmt.Errorf(getVisitedPath(path))
		}
		visited[currentName+"."+n.Task.Name] = true
		if err := visit(n.Task.Name, n.Prev, path, visited); err != nil {
			return err
		}
	}
	return nil
}

func getVisitedPath(path []string) string {
	// Reverse the path since we traversed the graph using prev pointers.
	for i := len(path)/2 - 1; i >= 0; i-- {
		opp := len(path) - 1 - i
		path[i], path[opp] = path[opp], path[i]
	}
	return strings.Join(path, " -> ")
}

// GetSchedulable returns a map of PipelineTask that can be scheduled (keyed
// by the name of the PipelineTask) given a list of successfully finished doneTasks.
// It returns tasks which have all dependecies marked as done, and thus can be scheduled. If the
// specified doneTasks are invalid (i.e. if it is indicated that a Task is
// done, but the previous Tasks are not done), an error is returned.
func (g *DAG) GetSchedulable(doneTasks ...string) (map[string]v1alpha1.PipelineTask, error) {
	roots := g.getRoots()
	tm := toMap(doneTasks...)
	d := map[string]v1alpha1.PipelineTask{}

	visited := map[string]struct{}{}
	for _, root := range roots {
		schedulable := findSchedulable(root, visited, tm)
		for _, task := range schedulable {
			d[task.Name] = task
		}
	}

	visitedNames := make([]string, len(visited))
	for v := range visited {
		visitedNames = append(visitedNames, v)
	}

	notVisited := list.DiffLeft(doneTasks, visitedNames)
	if len(notVisited) > 0 {
		return map[string]v1alpha1.PipelineTask{}, fmt.Errorf("invalid list of done tasks; some tasks were indicated completed without ancestors being done: %v", notVisited)
	}

	return d, nil
}

func (g *DAG) getRoots() []*Node {
	n := []*Node{}
	for _, node := range g.Nodes {
		if len(node.Prev) == 0 {
			n = append(n, node)
		}
	}
	return n
}

func findSchedulable(n *Node, visited map[string]struct{}, doneTasks map[string]struct{}) []v1alpha1.PipelineTask {
	if _, ok := visited[n.Task.Name]; ok {
		return []v1alpha1.PipelineTask{}
	}
	visited[n.Task.Name] = struct{}{}
	if _, ok := doneTasks[n.Task.Name]; ok {
		schedulable := []v1alpha1.PipelineTask{}
		// This one is done! Take note of it and look at the next candidate
		for _, next := range n.Next {
			if _, ok := visited[next.Task.Name]; !ok {
				schedulable = append(schedulable, findSchedulable(next, visited, doneTasks)...)
			}
		}
		return schedulable
	}
	// This one isn't done! Return it if it's schedulable
	if isSchedulable(doneTasks, n.Prev) {
		return []v1alpha1.PipelineTask{n.Task}
	}
	// This one isn't done, but it also isn't ready to schedule
	return []v1alpha1.PipelineTask{}
}

func isSchedulable(doneTasks map[string]struct{}, prevs []*Node) bool {
	if len(prevs) == 0 {
		return true
	}
	collected := []string{}
	for _, n := range prevs {
		if _, ok := doneTasks[n.Task.Name]; ok {
			collected = append(collected, n.Task.Name)
		}
	}
	return len(collected) == len(prevs)
}

func toMap(t ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(t))
	for _, s := range t {
		m[s] = struct{}{}
	}
	return m
}

func addLink(pt v1alpha1.PipelineTask, previousTask string, nodes map[string]*Node) error {
	prev, ok := nodes[previousTask]
	if !ok {
		return fmt.Errorf("Task %s depends on %s but %s wasn't present in Pipeline", pt.Name, previousTask, previousTask)
	}
	next, _ := nodes[pt.Name]
	if err := linkPipelineTasks(prev, next); err != nil {
		return fmt.Errorf("Couldn't create link from %s to %s: %v", prev.Task.Name, next.Task.Name, err)
	}
	return nil
}

// Build returns a valid pipeline DAG. Returns error if the pipeline is invalid
func Build(tasks []v1alpha1.PipelineTask) (*DAG, error) {
	d := new()

	// Add all Tasks mentioned in the `PipelineSpec`
	for _, pt := range tasks {
		if _, err := d.addPipelineTask(pt); err != nil {
			return nil, fmt.Errorf("task %s is already present in graph, can't add it again: %v", pt.Name, err)
		}
	}
	// Process all from and runAfter constraints to add task dependency
	for _, pt := range tasks {
		for _, previousTask := range pt.RunAfter {
			if err := addLink(pt, previousTask, d.Nodes); err != nil {
				return nil, fmt.Errorf("couldn't add link between %s and %s: %v", pt.Name, previousTask, err)
			}
		}
		if pt.Resources != nil {
			for _, rd := range pt.Resources.Inputs {
				for _, previousTask := range rd.From {
					if err := addLink(pt, previousTask, d.Nodes); err != nil {
						return nil, fmt.Errorf("couldn't add link between %s and %s: %v", pt.Name, previousTask, err)
					}
				}
			}
		}
	}
	return d, nil
}
