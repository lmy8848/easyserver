package model

import "easyserver/internal/container"

// Domain types moved to easyserver/internal/container.
// Kept as aliases for backward compatibility.

type Container = container.Container
type PortMapping = container.PortMapping
type Mount = container.Mount
type Image = container.Image
type CreateContainerRequest = container.CreateRequest
type DockerStatus = container.DockerStatus
type ContainerStats = container.Stats
type ContainerProcessInfo = container.ProcessInfo
type UpdateContainerRequest = container.UpdateRequest
type ComposeProject = container.ComposeProject
type Volume = container.Volume
type Network = container.Network
