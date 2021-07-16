package main

import (
	"context"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Container struct {
	Name      string
	Container types.Container
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	log.Info().Str("stage", "container").Msg("gathering container")
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		log.Fatal().Str("stage", "container").Msg(err.Error())
	}

	log.Info().Str("stage", "images").Msg("gathering images")
	images, err := cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		log.Fatal().Str("stage", "images").Msg(err.Error())
	}

	running := map[string][]Container{}
	for _, container := range containers {
		name := container.Names[0]
		if strings.HasPrefix(name, "/") {
			name = strings.Split(name, "/")[1]
		}

		running[container.Image] = append(running[container.Image], Container{Name: name, Container: container})
	}
	for _, image := range images {
		for _, tag := range image.RepoTags {
			if len(running[tag]) > 0 {
				log.Info().Str("stage", "pull").Msg(tag)
				id := image.ID
				out, err := cli.ImagePull(ctx, tag, types.ImagePullOptions{})
				if err != nil {
					log.Error().Str("stage", "pull").Msgf("can't pull %s", tag)
				} else {

					defer out.Close()

					//io.Copy(St, out)

					f := filters.KeyValuePair{}
					f.Key = "reference"
					f.Value = tag
					i, err := cli.ImageList(ctx, types.ImageListOptions{Filters: filters.NewArgs(f)})
					if err != nil {
						log.Fatal().Str("stage", "get image").Msg(err.Error())
					}
					for _, x := range i {
						for _, t := range x.RepoTags {
							if t == tag {
								if x.ID != id {
									for _, c := range running[tag] {
										log.Info().Str("stage", "pull").Msgf("pulled new image for %s\n", tag)
										conf, err := cli.ContainerInspect(ctx, c.Container.ID)
										if err != nil {
											log.Fatal().Str("stage", "get config").Msg(err.Error())
										}
										if err := cli.ContainerRemove(ctx, c.Container.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
											log.Fatal().Str("stage", "remove container").Msg(err.Error())
										}
										log.Info().Str("stage", "stop").Msgf("Successfully stopped %s\n", c.Name)
										if _, ok := c.Container.Labels["com.docker.stack.namespace"]; !ok {
											resp, err := cli.ContainerCreate(ctx, conf.Config, conf.HostConfig, &network.NetworkingConfig{EndpointsConfig: conf.NetworkSettings.Networks}, nil, c.Name)
											if err != nil {
												log.Fatal().Str("stage", "recreate").Msg(err.Error())
											}

											if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
												log.Fatal().Str("stage", "recreate").Msg(err.Error())
											}
											log.Info().Str("stage", "recreate").Msgf("created new container %s with image %s\n", c.Name, tag)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}
