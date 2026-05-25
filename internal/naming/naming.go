package naming

import (
	"fmt"
	"hash/fnv"
	"net"
	"strings"
)

var OrderedNames = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa"}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func Addr(name string) string {
	return fmt.Sprintf("%s:%d", GetLocalIP(), Port(name))
}

func Port(name string) int {
	switch strings.ToLower(name) {
	case "admin":
		return 4000
	case "alpha":
		return 4001
	case "beta":
		return 4002
	case "gamma":
		return 4003
	case "delta":
		return 4004
	case "epsilon":
		return 4005
	case "zeta":
		return 4006
	case "eta":
		return 4007
	case "theta":
		return 4008
	case "iota":
		return 4009
	case "kappa":
		return 4010
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	return 4100 + int(h.Sum32()%800)
}

func ConnectTarget(value string) string {
	if strings.Contains(value, ":") {
		return value
	}
	return Addr(value)
}

func FirstAvailableName() string {
	for _, name := range OrderedNames {
		if portAvailable(Port(name)) {
			return name
		}
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("node%d", i)
		if portAvailable(Port(name)) {
			return name
		}
	}
}

func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
