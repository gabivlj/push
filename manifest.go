package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ImageConfigRootfs struct {
	DiffIDs []string `json:"diff_ids"`
	Type    string   `json:"type"`
}

type ImageConfig struct {
	Rootfs ImageConfigRootfs `json:"rootfs"`
}

type ConfigManifest struct {
	// This descriptor property has additional restrictions for config.
	// Implementations MUST NOT attempt to parse the referenced content if this media type is unknown and instead consider the referenced content as arbitrary binary data (e.g.: as application/octet-stream).
	// Implementations storing or copying image manifests MUST NOT error on encountering a value that is unknown to the implementation
	// Implementations MUST support at least the following media types:
	//  - application/vnd.oci.image.config.v1+json
	// Manifests for container images concerned with portability SHOULD use one of the above media types. Manifests for artifacts concerned with portability SHOULD use config.mediaType as described in Guidelines for Artifact Usage.
	// If the manifest uses a different media type than the above, it MUST comply with RFC 6838, including the naming requirements in its section 4.2, and MAY be registered with IANA.
	MediaType string `json:"mediaType"`
	Size      uint64 `json:"size"`
	Digest    string `json:"digest"`
}

type LayerManifest struct {
	// This descriptor property has additional restrictions for layers[]. Implementations MUST support at least the following media types:
	// application/vnd.oci.image.layer.v1.tar
	// application/vnd.oci.image.layer.v1.tar+gzip
	// application/vnd.oci.image.layer.nondistributable.v1.tar
	// application/vnd.oci.image.layer.nondistributable.v1.tar+gzip
	// Manifests concerned with portability SHOULD use one of the above media types. Implementations storing or copying image manifests MUST NOT error on encountering a mediaType that is unknown to the implementation.
	// Entries in this field will frequently use the +gzip types.
	MediaType string `json:"mediaType"`
	Size      uint64 `json:"size"`
	Digest    string `json:"digest"`
}

type Manifest struct {
	// This REQUIRED property specifies the image manifest schema version. For this version of the specification,
	// this MUST be 2 to ensure backward compatibility with older versions of Docker. The value of this field will not change.
	// This field MAY be removed in a future version of the specification.
	SchemaVersion int `json:"schemaVersion"`

	// This property SHOULD be used and remain compatible with earlier versions of this specification and with other similar external formats.
	// When used, this field MUST contain the media type application/vnd.oci.image.manifest.v1+json.
	// This field usage differs from the descriptor use of mediaType.
	MediaType string `json:"mediaType"`

	// This OPTIONAL property contains the type of an artifact when the manifest is used for an artifact. This MUST be set when config.mediaType is set to the empty value. If defined, the value MUST comply with RFC 6838, including the naming requirements in its section 4.2, and MAY be registered with IANA.
	// Implementations storing or copying image manifests MUST NOT error on encountering an artifactType that is unknown to the implementation.
	ArtifactType string `json:"artifactType,omitempty"`

	// This REQUIRED property references a configuration object for a container, by digest.
	Config ConfigManifest `json:"config"`

	// Each item in the array MUST be a descriptor.
	// For portability, layers SHOULD have at least one entry. See the guidance for an empty descriptor below,
	// and DescriptorEmptyJSON of the reference code.
	Layers []LayerManifest `json:"layers"`
}

const storageDriver = "overlay2"

type RepositoryTag string

func (r RepositoryTag) getDockerPath() string {
	return fmt.Sprintf("/var/lib/docker/image/overlay2/imagedb/content/sha256/%s", strings.Split(string(r), ":")[1])
}

func (r RepositoryTag) IntoImageConfig(folder string) (ImageConfig, int64, error) {
	p := r.getDockerPath()
	fd, err := os.Open(p)
	if err != nil {
		return ImageConfig{}, 0, fmt.Errorf("open %q: %w", p, err)
	}

	defer fd.Close()
	fdLayer, err := os.Create(filepath.Join(folder, string(r)))
	if err != nil {
		return ImageConfig{}, 0, fmt.Errorf("create layers %q: %w", r, err)
	}

	defer fdLayer.Close()
	reader := io.TeeReader(fd, fdLayer)
	decoder := json.NewDecoder(reader)
	config := ImageConfig{}
	if err := decoder.Decode(&config); err != nil {
		return ImageConfig{}, 0, fmt.Errorf("decode: %w", err)
	}

	size := decoder.InputOffset()
	return config, size, nil
}

type Repository = map[string]RepositoryTag

type Repositories struct {
	Repositories map[string]Repository `json:"Repositories"`
}

func (r Repositories) getConfigSHA256(name, tag string) (RepositoryTag, error) {
	repos, ok := r.Repositories[name]
	if !ok {
		return "", fmt.Errorf("name %q not found", name)
	}

	fullName := fmt.Sprintf("%s:%s", name, tag)
	sha, ok := repos[fullName]
	if !ok {
		return "", fmt.Errorf("full tag %q not found", fullName)
	}

	return sha, nil
}

const layerFolder = "/var/lib/push/layers"

func generateManifestFromDocker(ctx context.Context, imageURL string, db *db) (Manifest, error) {
	if err := os.MkdirAll(layerFolder, os.ModePerm); err != nil {
		return Manifest{}, fmt.Errorf("layers: %w", err)
	}

	repositories, err := getRepositories(ctx)
	if err != nil {
		return Manifest{}, fmt.Errorf("get repositories: %w", err)
	}

	i := strings.LastIndex(imageURL, ":")
	if i == -1 {
		return Manifest{}, fmt.Errorf("unexpected ':' not found on %s", imageURL)
	}

	sha, err := repositories.getConfigSHA256(imageURL[:i], imageURL[i+1:])
	if err != nil {
		return Manifest{}, fmt.Errorf("get config: %w", err)
	}

	config, configSize, err := sha.IntoImageConfig(layerFolder)
	if err != nil {
		return Manifest{}, fmt.Errorf("image config: %w", err)
	}

	layers := make([]LayerReader, 0, len(config.Rootfs.DiffIDs))
	var prevNode *node
	wg := sync.WaitGroup{}
	for _, layer := range config.Rootfs.DiffIDs {
		wg.Add(1)
		if prevNode == nil {
			prevNode = db.Get(layer)
		} else {
			prevNode = db.GetChild(prevNode, layer)
		}

		p := filepath.Join(layerFolder, layer)
		if stat, err := os.Stat(p); err == nil && stat != nil {
			layers = append(layers, noopLayer{size: uint64(stat.Size()), hash: layer})
			continue
		}

		reader, err := newLayerReader(prevNode)
		if err != nil {
			return Manifest{}, fmt.Errorf("create layer reader %q: %w", layer, err)
		}

		layers = append(layers, reader)
		defer reader.Close()
	}

	errs := make(chan error, len(layers))
	for i, layer := range layers {
		if _, isNoop := layer.(noopLayer); isNoop {
			wg.Done()
			continue
		}

		i, layer := i, layer
		go func() {
			defer wg.Done()
			p := filepath.Join(layerFolder, config.Rootfs.DiffIDs[i])
			fd, err := os.Create(p)
			if err != nil {
				errs <- fmt.Errorf("creating file %s: %w", config.Rootfs.DiffIDs[i], err)
				return
			}

			if _, err := io.Copy(fd, layer); err != nil {
				errs <- fmt.Errorf("copying layer %s: %w", config.Rootfs.DiffIDs[i], err)
			}
		}()
	}

	wg.Wait()
	containsErr := false
forLoop:
	for {
		select {
		case err := <-errs:
			containsErr = true
			fmt.Fprintln(os.Stderr, "Error", err.Error())
		default:
			break forLoop
		}
	}

	if containsErr {
		return Manifest{}, errors.New("there has been errors, see stderr output")
	}

	m := Manifest{}
	m.Config = ConfigManifest{
		MediaType: "application/vnd.oci.image.config.v1+json",
		Size:      uint64(configSize),
		Digest:    string(sha),
	}
	m.Layers = make([]LayerManifest, 0, len(layers))
	for _, layer := range layers {
		m.Layers = append(m.Layers, LayerManifest{
			Size:      layer.Size(),
			Digest:    layer.Hash(),
			MediaType: "application/vnd.oci.image.layer.v1.tar",
		})
	}

	m.SchemaVersion = 2
	m.MediaType = "application/vnd.oci.image.manifest.v1+json"
	return m, nil
}

func getRepositoriesPath() string {
	return fmt.Sprintf("/var/lib/docker/image/%s/repositories.json", storageDriver)
}

func getRepositories(ctx context.Context) (Repositories, error) {
	p := getRepositoriesPath()
	fd, err := os.Open(p)
	if err != nil {
		return Repositories{}, fmt.Errorf("open %q: %w", p, err)
	}

	defer fd.Close()
	decoder := json.NewDecoder(fd)
	r := Repositories{}
	if err := decoder.Decode(&r); err != nil {
		return r, fmt.Errorf("decode: %w", err)
	}

	return r, nil
}
