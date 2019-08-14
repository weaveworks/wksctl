package kubeadm

import (
	"strings"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
)

const (
	kubeadmJoin = "kubeadm join"
	empty       = ""
	newline     = "\n"
	whitespace  = " "
	backslash   = "\\"
	// tokenDiscoveryCAHash flag instruct kubeadm to validate that the root CA public key matches this hash (for token-based discovery)
	tokenDiscoveryCAHash = "discovery-token-ca-cert-hash"
	// certificateKey flag sets the key used to encrypt and decrypt certificate secrets
	certificateKey = "certificate-key"
)

// ExtractJoinCmd goes through the provided kubeadm init standard output and
// extracts the kubeadm join command printed.
func ExtractJoinCmd(stdOut string) (string, error) {
	// Another way to get the kubeadm join command is by creating another token
	// using: kubeadm token create --print-join-command
	// but given we cannot conveniently remove the previous token, via the CLI,
	// for now, we instead parse the output of kubeadm init to extract the join
	// command.
	lines := strings.Split(stdOut, newline)
	withinCmd := false
	var cmd strings.Builder
	for _, line := range lines {
		if strings.Contains(line, kubeadmJoin) { // Beginning of the command.
			cmd.WriteString(sanitize(line))
			if hasLineContinuation(line) {
				cmd.WriteString(whitespace)
				withinCmd = true
			} else {
				break
			}
		} else if withinCmd {
			cmd.WriteString(sanitize(line))
			if hasLineContinuation(line) {
				cmd.WriteString(whitespace)
			} else {
				break
			}
		}
	}
	if cmd.Len() > 0 {
		return cmd.String(), nil
	}
	return "", errors.New("kubeadm join command not found")
}

func sanitize(line string) string {
	line = strings.TrimRight(line, backslash) // Remove line continuation.
	line = strings.TrimSpace(line)
	return line
}

func hasLineContinuation(line string) bool {
	return strings.HasSuffix(line, backslash)
}

// ExtractDiscoveryTokenCaCertHash extracts the discover token CA cert hash
// from the provided kubeadm join command.
func ExtractDiscoveryTokenCaCertHash(kubeadmJoinCmd string) (string, error) {
	return extractFlag(kubeadmJoinCmd, tokenDiscoveryCAHash, "discovery token CA cert hash not found")
}

// ExtractCertificateKey extracts the certificate key from the provided kubeadm
// join command.
func ExtractCertificateKey(kubeadmJoinCmd string) (string, error) {
	return extractFlag(kubeadmJoinCmd, certificateKey, "certificate key not found")
}

func extractFlag(kubeadmJoinCmd, name, errorMessage string) (string, error) {
	cmd := strings.Split(kubeadmJoinCmd, whitespace)
	flagSet := flag.NewFlagSet(cmd[0], flag.ContinueOnError)
	value := flagSet.String(name, empty, empty)
	flagSet.ParseErrorsWhitelist.UnknownFlags = true // Ignore other flags.
	if err := flagSet.Parse(cmd[1:]); err != nil {
		return "", errors.Wrap(err, errorMessage)
	}
	return *value, nil
}
