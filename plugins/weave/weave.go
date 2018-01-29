package weave

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
)

const (
	weavePath      = "/usr/local/bin/weave"
	weaveHTTPAddr  = "127.0.0.1:6784"
	defaultBaseURL = "http://127.0.0.1:6784"
)

var (
	logger     = logrus.New()
	httpClient = &http.Client{
		Timeout: time.Second * 3,
	}
)

func (c *Config) DNSDomainDotted() string {
	domain := strings.Trim(c.DNSDomain, ".")
	return fmt.Sprintf("%s.", domain)
}

func (w *Weave) cli(env []string, args ...string) ([]byte, error) {
	cmd := exec.Command(weavePath, args...)
	for _, e := range env {
		cmd.Env = append(cmd.Env, e)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	return []byte(out), nil
}

// Verify checks that the ip-alloc-range and other required config are correctly configured in weave
func (w *Weave) Verify(status *Status) error {
	if w.Config.IPAllocRange != status.IPAM.Range {
		return fmt.Errorf("Config IPAllocRange not matching active weave: %s!=%s", w.Config.IPAllocRange, status.IPAM.Range)
	}

	if w.Config.DNSDomainDotted() != status.DNS.Domain {
		return fmt.Errorf("Config DNSDomain not matching active weave: %s!=%s", w.Config.DNSDomainDotted(), status.DNS.Domain)
	}
	return nil
}

// Auto ensures weave is running with a correct configuration
func (w *Weave) Auto() (*Status, error) {
	if err := w.IsRunning(); err != nil {
		logger.Debug(err)
		logger.Info("Weave is not running, starting weave...")
		w.Launch()
		return w.Auto()
	}
	logger.Info("Weave is running")
	logger.Info("Checking configuration")
	status, err := w.Status()
	if err != nil {
		return nil, err
	}
	if err := w.Verify(status); err != nil {
		logger.Info(err)
		logger.Info("Current weave config differs, resetting weave")
		w.Destroy()
		logger.Info("Weave destroyed")
		return w.Auto()
	}
	logger.Info("Current weave config OK")
	return status, nil
}

// Destroy clear the weave instance
func (w *Weave) Destroy() error {
	_, err := w.cli(nil,
		"reset",
		"--force",
	)
	if err != nil {
		return err
	}
	return nil
}

// Launch starts weave
func (w *Weave) Launch() {
	env := []string{"CHECKPOINT_DISABLE=1"}
	args := []string{
		"launch",
		"--password",
		w.Config.Password,
		"--ipalloc-range",
		w.Config.IPAllocRange,
		"--dns-domain",
		w.Config.DNSDomain,
		"--plugin=false",
		"--proxy=false",
		"--no-discovery",
	}
	if w.Debug {
		args = append(args, "--log-level=debug")
	}
	w.cli(env, args...)
}

// DNSAdd adds a dns entry in weave dns server
func (w *Weave) DNSAdd(containerID string, ip string, entry string) error {
	_, err := w.cli(nil,
		"dns-add",
		ip,
		containerID,
		"-h",
		entry,
	)
	if err != nil {
		return err
	}
	return nil
}

// DNSRemove removes a dns entry from weave dns server
func (w *Weave) DNSRemove(containerID string, ip string) error {
	_, err := w.cli(nil,
		"dns-remove",
		ip,
		containerID,
	)
	if err != nil {
		return err
	}
	return nil
}

// ContainerDNS returns the weave DNS IP for use in containers
func (w *Weave) ContainerDNS() (string, error) {
	status, err := w.Status()
	if err != nil {
		return "", err
	}
	s := strings.Split(status.DNS.Address, ":")
	return s[0], nil
}

// Attach a container to weave
func (w *Weave) Attach(cid string, network string) (string, error) {
	ip, err := w.cli(nil,
		"attach",
		fmt.Sprintf("net:%s", network),
		cid,
	)
	if err != nil {
		return "", fmt.Errorf("%s %s", err, ip)
	}
	return string(ip), nil
}

// TestTCPConnection tries to connect to a remote weave router
func (w *Weave) TestTCPConnection(ip string) error {
	dialer := net.Dialer{
		Timeout: 3 * time.Second,
	}
	conn, err := dialer.Dial("tcp", fmt.Sprintf("%s:6783", ip))
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

// Connect a peer to weave
func (w *Weave) Connect(ip string) error {
	_, err := w.cli(nil,
		"connect",
		ip,
	)
	if err != nil {
		return err
	}
	return nil
}

// Forget a peer from weave
func (w *Weave) Forget(ip string) error {
	_, err := w.cli(nil,
		"forget",
		ip,
	)
	if err != nil {
		return err
	}
	return nil
}

// Detach a container from weave
func (w *Weave) Detach(cid string, network string) ([]string, error) {
	ipsList, err := w.cli(nil,
		"detach",
		fmt.Sprintf("net:%s", network),
		cid,
	)
	if err != nil {
		return nil, err
	}
	detachedIPs := strings.Split(string(ipsList), " ")
	return detachedIPs, nil
}

// NewWeave returns a new weave instance
func NewWeave(dnsdomain string, ipallocRange string, password string, debug bool) *Weave {
	// ensure weave script is present
	_, err := os.Stat(weavePath)
	if err != nil {
		log.Fatal(err)
	}

	w := &Weave{
		Debug: debug,
		Config: &Config{
			DNSDomain:    dnsdomain,
			IPAllocRange: ipallocRange,
			Password:     password,
		},
		HTTPClient: httpClient,
		BaseURL:    defaultBaseURL,
	}
	if _, err := w.cli(nil, "version"); err != nil {
		log.Fatalf("Cannot get weave version: %s", err)
	}
	return w
}

// IsRunning check if weave container is running
func (w *Weave) IsRunning() error {
	_, err := w.Status()
	if err != nil {
		return err
	}
	return nil
}
