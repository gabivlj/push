package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ID = string

type DiffID = string

type db struct {
	allNodes map[ID]*node
}

func (d *db) Get(id ID) *node {
	return d.allNodes[id]
}

func (d *db) GetChild(node *node, diffID DiffID) *node {
	id := node.children[diffID]
	return d.Get(id)
}

func (d *db) Insert(n *node) {
	if prevNode, ok := d.allNodes[n.id]; ok {
		n.children = prevNode.children
	} else {
		n.children = map[string]string{}
	}

	d.allNodes[n.id] = n
	if parentNode, ok := d.allNodes[n.parent]; ok {
		parentNode.children[n.diffID] = n.id
	} else {
		d.allNodes[n.parent] = &node{
			children: map[string]string{
				n.diffID: n.id,
			},
		}
	}
}

func newDb() (*db, error) {
	d := &db{
		allNodes: map[string]*node{},
	}
	basePath := filepath.Join("/var", "lib", "docker", "image", storageDriver, "layerdb", "sha256")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", basePath, err)
	}

	for _, entry := range entries {
		p := filepath.Join(basePath, entry.Name())
		parentPath := filepath.Join(p, "parent")
		parent, err := os.ReadFile(parentPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read parent file %q: %w", parentPath, err)
		}

		if parent == nil {
			parent = []byte("")
		}

		diffIDPath := filepath.Join(p, "diff")
		diffID, err := os.ReadFile(diffIDPath)
		if err != nil {
			return nil, fmt.Errorf("read diffID file %q: %w", parentPath, err)
		}

		cacheIDPath := filepath.Join(p, "cache-id")
		cacheID, err := os.ReadFile(cacheIDPath)
		if err != nil {
			return nil, fmt.Errorf("read cache-id file %q: %w", cacheIDPath, err)
		}

		n := &node{
			cacheID: string(cacheID),
			diffID:  string(diffID),
			parent:  string(parent),
			id:      fmt.Sprintf("sha256:%s", entry.Name()),
		}
		d.Insert(n)
	}

	return d, nil
}

type node struct {
	id       ID
	diffID   DiffID
	cacheID  string
	parent   ID
	children map[DiffID]ID
}
