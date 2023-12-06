/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

func main() {
	if err := checkDrone(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var pipelines []pipeline

	pipelines = append(pipelines, pushPipelines()...)
	pipelines = append(pipelines, tagPipelines()...)
	pipelines = append(pipelines, cronPipelines()...)
	pipelines = append(pipelines, promoteBuildPipelines()...)
	pipelines = append(pipelines, updateDocsPipeline())
	pipelines = append(pipelines, buildboxPipeline())
	pipelines = append(pipelines, buildContainerImagePipelines()...)
	pipelines = append(pipelines, publishReleasePipeline())

	// Inject the Drone-level dockerhub credentials into all non-exec
	// pipelines. Drone will then use the docker credentials file in
	// the named secret as its credentials when pulling images from
	// dockerhub.
	//
	// Exec pipelines do not have the `image_pull_secrets` option, as
	// their steps are invoked directly on the host runner and not
	// into a per-step container.
	for pidx := range pipelines {
		p := &pipelines[pidx]
		if p.Type == "exec" {
			continue
		}
		p.ImagePullSecrets = append(p.ImagePullSecrets, "DOCKERHUB_CREDENTIALS")
	}

	if err := writePipelines(".drone.yml", pipelines); err != nil {
		fmt.Println("failed writing drone pipelines:", err)
		os.Exit(1)
	}

	if err := signDroneConfig(); err != nil {
		fmt.Println("failed signing .drone.yml:", err)
		os.Exit(1)
	}
}

func writePipelines(path string, newPipelines []pipeline) error {
	// Read the existing config and replace only those pipelines defined in
	// newPipelines.
	//
	// TODO: When all pipelines are migrated, remove this merging logic and
	// write the file directly. This will be simpler and allow cleanup of
	// pipelines when they are removed from this generator.
	existingConfig, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read existing config: %w", err)
	}
	existingPipelines, err := parsePipelines(existingConfig)
	if err != nil {
		return fmt.Errorf("failed to parse existing config: %w", err)
	}

	newPipelinesSet := make(map[string]pipeline, len(newPipelines))
	for _, p := range newPipelines {
		// TODO: remove this check once promoteBuildPipeline and
		// updateDocsPipeline are implemented.
		if p.Name == "" {
			continue
		}
		newPipelinesSet[p.Name] = p
	}

	pipelines := existingPipelines
	// Overwrite all existing pipelines with new ones that have the same name.
	for i, p := range pipelines {
		if np, ok := newPipelinesSet[p.Name]; ok {
			out, err := yaml.Marshal(np)
			if err != nil {
				return fmt.Errorf("failed to encode pipelines: %w", err)
			}
			// Add a little note about this being generated.
			out = append([]byte(np.comment), out...)
			pipelines[i] = parsedPipeline{pipeline: np, raw: out}
			delete(newPipelinesSet, np.Name)
		}
	}
	// If we decide to add new pipelines before everything is migrated to this
	// generator, this check needs to change.
	if len(newPipelinesSet) != 0 {
		var names []string
		for n := range newPipelinesSet {
			names = append(names, n)
		}
		return fmt.Errorf("pipelines %q don't exist in the current config, aborting", names)
	}

	var pipelinesEnc [][]byte
	for _, p := range pipelines {
		pipelinesEnc = append(pipelinesEnc, p.raw)
	}
	configData := bytes.Join(pipelinesEnc, []byte("\n---\n"))

	return os.WriteFile(path, configData, 0664)
}

// parsedPipeline is a single pipeline parsed from .drone.yml along with its
// unparsed form. It's used to preserve YAML comments and minimize diffs due to
// formatting.
//
// TODO: remove this when all pipelines are migrated. All comments will be
// moved to this generator instead.
type parsedPipeline struct {
	pipeline
	raw []byte
}

func parsePipelines(data []byte) ([]parsedPipeline, error) {
	chunks := bytes.Split(data, []byte("\n---\n"))
	var pipelines []parsedPipeline
	for _, c := range chunks {
		// Discard the signature, it will be re-generated.
		if bytes.HasPrefix(c, []byte("kind: signature")) {
			continue
		}
		var p pipeline
		if err := yaml.UnmarshalStrict(c, &p); err != nil {
			return nil, err
		}
		pipelines = append(pipelines, parsedPipeline{pipeline: p, raw: c})
	}
	return pipelines, nil
}
