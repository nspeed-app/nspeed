package nspeed

// mainly a copy/paste of https://github.com/joeshaw/cipherlist

// Using go:linkname requires us to import unsafe
import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"time"
	_ "unsafe" //

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/cpu"
	"nspeed.app/metrics"
	"nspeed.app/utils"
)

// from: https://www.joeshaw.org/abusing-go-linkname-to-customize-tls13-cipher-suites/

// We bring the real defaultCipherSuitesTLS13 function from the
// crypto/tls package into our own package.  This lets us perform
// that lazy initialization of the cipher list when we want.

// 17 aug 2021: updated for go 1.17 by jfgiorgi:
//   - defaultCipherSuitesTLS13 func is gone and replaced by 2 arrays:
//   - defaultCipherSuitesTLS13
//   - defaultCipherSuitesTLS13NoAES
//     depending on hardware features (see hasAESGCMHardwareSupport in crypto/tls/cipher_suites.go )
var (
	hasGCMAsmAMD64 = cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ
	hasGCMAsmARM64 = cpu.ARM64.HasAES && cpu.ARM64.HasPMULL
	// Keep in sync with crypto/aes/cipher_s390x.go.
	hasGCMAsmS390X = cpu.S390X.HasAES && cpu.S390X.HasAESCBC && cpu.S390X.HasAESCTR &&
		(cpu.S390X.HasGHASH || cpu.S390X.HasAESGCM)

	HasAESGCMHardwareSupport = runtime.GOARCH == "amd64" && hasGCMAsmAMD64 ||
		runtime.GOARCH == "arm64" && hasGCMAsmARM64 ||
		runtime.GOARCH == "s390x" && hasGCMAsmS390X
)

//go:linkname defaultCipherSuitesTLS13 crypto/tls.defaultCipherSuitesTLS13
var defaultCipherSuitesTLS13 []uint16

//go:linkname defaultCipherSuitesTLS13NoAES crypto/tls.defaultCipherSuitesTLS13NoAES
var defaultCipherSuitesTLS13NoAES []uint16

// Also keep a variable around for the real default set, so we
// can reset it once we're finished.
var realDefaultCipherSuitesTLS13 []uint16
var realDefaultCipherSuitesTLS13NoAES []uint16

func init() {
	// backup actual real suites
	realDefaultCipherSuitesTLS13 = defaultCipherSuitesTLS13
	realDefaultCipherSuitesTLS13NoAES = defaultCipherSuitesTLS13NoAES
}

func supportedTLS13Ciphers(hostname, port string, timeout time.Duration) ([]uint16, error) {

	var supportedCiphers []uint16
	var ciphersToTest []uint16

	if HasAESGCMHardwareSupport {
		ciphersToTest = realDefaultCipherSuitesTLS13
	} else {
		ciphersToTest = realDefaultCipherSuitesTLS13NoAES
	}

	for _, c := range ciphersToTest {
		cfg := &tls.Config{
			ServerName: hostname,
			MinVersion: tls.VersionTLS13,
		}

		// Override the internal slices!
		defaultCipherSuitesTLS13 = []uint16{c}
		defaultCipherSuitesTLS13NoAES = []uint16{c}

		conn, err := net.DialTimeout("tcp", net.JoinHostPort(hostname, port), timeout)
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		_ = client.Handshake()
		client.Close()

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	// Reset the internal slice back to the full set
	defaultCipherSuitesTLS13 = realDefaultCipherSuitesTLS13
	defaultCipherSuitesTLS13NoAES = realDefaultCipherSuitesTLS13NoAES

	return supportedCiphers, nil
}

func supportedTLS12Ciphers(hostname, port string, timeout time.Duration) ([]uint16, error) {
	// Taken from https://golang.org/pkg/crypto/tls/#pkg-constants
	var allCiphers = []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	var supportedCiphers []uint16

	for _, c := range allCiphers {
		cfg := &tls.Config{
			ServerName:   hostname,
			CipherSuites: []uint16{c},
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   tls.VersionTLS12,
		}

		log.Debug().Msgf("dialing ...%s:%s (%s)", hostname, port, timeout.String())
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(hostname, port), timeout)
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		_ = client.Handshake()
		client.Close()

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	return supportedCiphers, nil
}

// implement Job
type JobCiphers struct {
	JobData
	Target  string
	Delay   uint
	host    string
	port    string
	Timeout time.Duration
}

func (j *JobCiphers) Init(_ Options) error {

	u, err := utils.ParseURL(j.Target, "https")
	if err != nil {
		return err
	}
	j.host = u.Hostname()
	j.port = u.Port()
	if j.port == "" {
		j.port = "443"
	}

	if j.Timeout == 0 {
		j.Timeout = 3 * time.Second
	}

	return nil
}

// print the ciphers list of hostname (dial address)
func (j *JobCiphers) Run(_ context.Context, wg *sync.WaitGroup, _ Options) (*metrics.Metric, error) {

	wm := &metrics.Metric{}
	wm.SetLogger(j.logger)

	defer func() {
		wm.Done = true
		wg.Done()
	}()

	if j.Delay > 0 {
		j.Logger().Debug().Uint("delay", j.Delay).Msg("waiting")
		time.Sleep(time.Duration(j.Delay) * time.Second)
		j.Logger().Debug().Msg("waking up")
	}

	log.Info().Bool("HardwareAES", HasAESGCMHardwareSupport).Send()
	fmt.Printf("this client has AES hardware support: %t\n", HasAESGCMHardwareSupport)

	range12, err := supportedTLS12Ciphers(j.host, j.port, j.Timeout)
	if err != nil {
		wm.Error = err
		return wm, err
	}
	LogCipher(j.host, j.port, utils.TLSVersionFriendlyName(tls.VersionTLS12), range12, j.Logger())

	range13, err := supportedTLS13Ciphers(j.host, j.port, j.Timeout)
	if err != nil {
		wm.Error = err
		return wm, err
	}
	LogCipher(j.host, j.port, utils.TLSVersionFriendlyName(tls.VersionTLS13), range13, j.Logger())

	return wm, nil
}

func (j *JobCiphers) GetTarget() string {
	return fmt.Sprintf("ciphers %s", j.Target)
}

func (j *JobCiphers) GoString() string {
	return fmt.Sprintf("ciphers {Target: %s (%s;%s), delay: %d, timeout:%s}",
		j.Target,
		j.host, j.port,
		j.Delay,
		j.Timeout,
	)
}

func (j *JobCiphers) PrintString(_, _ bool) string {
	return fmt.Sprintf("ciphers %s (%s:%s)", j.Target, j.host, j.port)
}

func (j *JobCiphers) IsInfinite() bool {
	return false
}

// print out the cipher list and target using a Logger
func LogCipher(host, port, tlsVersion string, ciphers []uint16, log *zerolog.Logger) {
	finalTarget := net.JoinHostPort(host, port)
	for _, c := range ciphers {
		log.Info().Msgf("%s supports %s %s", finalTarget, tlsVersion, tls.CipherSuiteName(c))
		fmt.Printf("%s supports %s %s\n", finalTarget, tlsVersion, tls.CipherSuiteName(c))
	}
	if len(ciphers) == 0 {
		log.Info().Msgf("%s doesn't support %s", finalTarget, tlsVersion)
		fmt.Printf("%s doesn't support %s\n", finalTarget, tlsVersion)
	}
}

func CmdCiphersParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(output)
	j := JobCiphers{}

	fs.UintVar(&j.Delay, "w", 0, "delay in seconds to wait before starting the command")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s: ", ciphersCommand)
		fmt.Fprintf(fs.Output(), "  %s hostname[:port]\n", ciphersCommand)
		fmt.Fprintf(fs.Output(), "argument is a hostname or an ip address with an optionnal port\n")
		fmt.Fprintf(fs.Output(), "if no port number is specified, 443 will be used\n")
		fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\n")
	}
	if err := fs.Parse(cmdargs); err != nil {
		return cmdargs, err
	}
	args := fs.Args()
	if len(args) < 1 {
		fs.Usage()
		return args, errors.New("missing target")
	}
	j.Target = args[0]
	args = args[1:]
	err := jl.AddJobs(&j)
	return args, err
}
