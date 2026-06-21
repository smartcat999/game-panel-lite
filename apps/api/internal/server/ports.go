package server

type PortAllocator interface {
	AllocateHostPort() (int, error)
}
