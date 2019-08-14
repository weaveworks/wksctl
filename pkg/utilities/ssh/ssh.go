package ssh

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
	"golang.org/x/crypto/ssh"
)

// ReadPrivateKey reads the provided private SSH key file and returns the
// corresponding bytes.
func ReadPrivateKey(privateKeyPath string) ([]byte, error) {
	privateKeyPath, err := path.Expand(privateKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to expand path to private key \"%s\"", privateKeyPath)
	}
	privateKey, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read private key \"%s\"", privateKeyPath)
	}
	return privateKey, nil
}

func SignerFromPrivateKey(privateKeyPath string, privateKey []byte) (ssh.Signer, error) {
	if len(privateKey) == 0 {
		var err error
		privateKey, err = ReadPrivateKey(privateKeyPath)
		if err != nil {
			return nil, err
		}
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse private key \"%s\"", privateKeyPath)
	}
	return signer, nil
}

func HostKeyCallback(hostPublicKey ssh.PublicKey) ssh.HostKeyCallback {
	if hostPublicKey == nil {
		return ssh.InsecureIgnoreHostKey()
	}
	return ssh.FixedHostKey(hostPublicKey)
}

func HostPublicKey(host string) (ssh.PublicKey, error) {
	path := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		// return a nil error, as this is logically equivalent to having a file
		// without any public key, or without this host's public key, i.e.:
		// we still want to connect to this server.
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	defer file.Close()
	return scanHostPublicKey(file, host)
}

func scanHostPublicKey(reader io.Reader, host string) (ssh.PublicKey, error) {
	scanner := bufio.NewScanner(reader)
	var hostKey ssh.PublicKey
	for scanner.Scan() {
		fields := strings.Split(sanitizeLine(scanner.Text()), " ")
		if len(fields) != 3 {
			continue
		}
		if strings.Contains(fields[0], host) {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				return nil, errors.Wrapf(err, "error parsing %q", fields[2])
			}
			break
		}
	}
	return hostKey, nil
}

func sanitizeLine(line string) string {
	line = strings.Trim(line, " \t")
	if strings.HasPrefix(line, "#") {
		// Skip comments.
		return ""
	}
	return line
}
