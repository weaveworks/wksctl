#!/bin/bash
#
# Description:
#   Helper functions to programmatically provision (e.g. for CIT).
#   Aliases on these functions are also created so that this script can be
#   sourced in your shell, in your ~/.bashrc file, etc. and directly called.
#
# Usage:
#   Source this file and call the relevant functions.
#

function ssh_public_key() {
    echo -e "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDJnmjlApwSXJ291Aua+w6+tBiuoluW3cqQHR9OE0NChg2e4mf96P/0WqUUB1UYiojr4OFCeDqgeR63ggd5YNcP5ul9TWgC6w9PhXZ3IlpRrNZyJILo1NvIdCKqwxVJSa6/hCIClsyLHicObxN58NCoH2LPpVtrsyIXczQRwycWcoYxfSmmHvnNvG6Vx54qXKvdIZiYI07XplA5jEA++9IrwHrU0pjDUitLUcYmVqtbNK29w4Bp1oVyjcgAp1dZputDc0XfepgJ/yCulIPAOS5/5cyDWZ3tvhHX6h/VugGhiqq9jMbh+z++ocGQlibY++oMxr7wTYm8p4e0Ax+JdvlsxxCsik4qRzUqZBFoxx+67rvSo0bqC87MtDn0QaCN6mZYT5yb9cw5ZeFfTyeCCz6T2mt2sBc+gQLFld67JWh+E8vthZmEXhSYMS09vHN4QmFcZK4MIbOwKzuZ5vW4NADESt+kWmOfppGk7jsXEnhmIbm2oi9q0CQwJ6q7aVegLhLGckF59Z/wCsnh/ND/55OzvUneMCNUzDp8KEMaj14sAiLvXBWDMgDdayeAMGjkbXlPJOQqAUwZoENPrw4qxiirUMGereAImq9KWEt/NBjXmyCgxZHZ/YkldxFDM9pe+GPcgk1PxDZXFMWn/vAKAAAnMtTQj+3yNtgJzhGeaiKKGw== wksctl-cit"
}

function decrypt() {
    if [ -z "$1" ]; then
        echo >&2 "Failed to decode and decrypt $2: no secret key was provided."
        return 1
    fi
    # Set md5 because existing keys were encrypted that way and openssl default changed
    echo "$3" | openssl base64 -d | openssl enc -md md5 -d -aes256 -pass "pass:$1"
}

function ssh_private_key() {
    # The private key has been AES256-encrypted and then Base64-encoded using the following command:
    #   $ openssl enc -in /tmp/wksctl_cit_id_rsa -e -aes256 -pass stdin | openssl base64 > /tmp/wksctl_cit_id_rsa.aes.b64
    # The below command does the reverse, i.e. base64-decode and AES-decrypt the file, and prints it to stdout.
    # N.B.: re-generate the SSH key using:
    #   $ ssh-keygen -t rsa -b 4096 -C "wksctl-cit"
    decrypt "$1" "SSH private key" "$(
        cat <<EOF
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
EOF
    )"
}

function set_up_ssh_private_key() {
    if [ -z "$1" ]; then
        echo >&2 "Failed to decode and decrypt SSH private key: no secret key was provided."
        return 1
    fi
    local ssh_private_key_path="$HOME/.ssh/wksctl_cit_id_rsa"
    [ -e "$ssh_private_key_path" ] && rm -f "$ssh_private_key_path"
    ssh_private_key "$1" >"$ssh_private_key_path"
    chmod 400 "$ssh_private_key_path"
    echo "$ssh_private_key_path"
}

function gcp_credentials() {
    # The below GCP service account JSON credentials have been AES256-encrypted and then Base64-encoded using the following command:
    #   $ openssl enc -in ~/.ssh/wksctl-cit.json -e -aes256 -pass stdin | openssl base64 > /tmp/wksctl-cit.json.aes.b64
    # The below command does the reverse, i.e. base64-decode and AES-decrypt the file, and prints it to stdout.
    # N.B.: Ask the password to Marc, or otherwise re-generate the credentials for GCP, as per ../tools/provisioning/gcp/README.md.
    decrypt "$1" "JSON credentials" "$(
        cat <<EOF
U2FsdGVkX1+B4lPjbXqZ0bskwKi9eVqSwNm/86JMd/nU6JRWf9zQftuAbUJlj4he
EoEIFXc8fywD+IgEqAXwXLfSSsNttmUAvQUzZ3HtDH712MiUTsio7qvDwUvInA6D
8pgVTVhd7SzeKF5O7j4Hb/4Pbx6EmYn19p72tKNc9FHA7hwpiccT+iTPh/v2XY2c
VSQ3gPkZQRatHwy1nyqLeeqL231QBlIKHviUd8kedCZrdoaFtK4FKG2BAyURKW1S
G8+HmYG6WVPKnwi7OU36gR6fGUkpfdo7poCTcNSA14vD76p/Og0VpIM4NeXzf6Ft
uKXGBgeCfaFZr/6/YjPAcs8igURJpoGo8kyxh0YbuJxK8XErCzktsXGTAIuXft0Y
h58s714SD/m74FZ4P5znjlBbqQOGKEqr2Eux4Y7XiN8F4HMdh7S1HrFmlerGmZLi
oX2+gZzLXfC21eOeASl/Pc4R8rK/IZeB5On8IRkye+DrYnlxE/elyRDe3wQpRwHE
hX1589mvcyyjP1UiUt5FZMbI3PZpMZk0t3/KZaUPAjslETGymCYjZ1dOP8Mg7+Be
Nqh9Hlu8wKLHTeaZ6j/vq+VBdh6DE3wZWJCWjPOiB67tgSSZy7Nt4rI1HsVIWVNG
qVYpUE/sci55BYcodHGY4mEc4oyBWheBwAGiuGH1/ewxD2MgnUB3GWVje18dQ6jl
r9Q8DJ7C9IfjQx8+QHd4cATbkw8ESf+P8pPLQodzIM2cMigeMFf3YHdGs+zM3yjo
Cdfj+yvYwMb1WEGc1yBwTYXBlX96Bc2dsEVee8Ik6bpwp7QHKg3ZtWG5Z7TQ5aNm
A6FUDlrp/PQe47a1QBlF9YMXJh0UN+q1ANhF6cH2RzeR9/kbpRocSuiWxNzuj0iX
xsY23D0psJInLo1zbSHytAfSPD2FW1hBMSsJJVFgvI+RxGh8WJ0M7Yu+975nlUZH
EyQAT8GCIgS5Gg155cR9bzJlTmm66MRr6aXkYu5/2AjECyc71e9kyxrtKfKmng3j
b6T2X57A+jOKFtxCj5ul+uEvUOMl14/1z+a5iplso8umoFDj5k7GinTsobXCX3Mx
VgJ86N1weOzbaany4vvyWet3MVuCODyWaPRvrtXMHyuHH184SsM2glXzJt1+qglQ
3kCzMESy4H8L3kNnte2bbX5trR4rSaa7vCi1f88LP/gBonSuMkGxulRA6xfjyIYk
12l/IWKCaYWrr1RWlgwMWolH4VR79pc8Api5gkjzMmetxajNaJhiVbCJ2SM0hcH7
TUQI8t/sp0ApZDSM2zS9HmJYm3zEbOl5Ieg0845oz4hBSC0V8clevMIebDG0uXnW
X+j9HsVbYbaJvC21n/wQS5OZDCLyt4aKHRxsolR/g97FYV2qyC194QTE5PImlvw4
2fbIRr0GGYBj5q7lcFlNMIPocQid1nQux9fyOjRt4zlZpvMR9oMEqh07D6XIWmO0
OiQskssNKEzU8CxMRjDNoT7DhdShTwhQ0Z62RM6qXkxda0xmvE5wTO8yV9UaJH8V
dJABKHYOay0vQrxzHg2FL22IiXOMYcOzPn9RgLhmghpQnT9ZTAA6jEDI4pAyrm4O
61FeC5uFLHr4cQYAGc6QQbghgIJyhL7ESuaSeVs3qjdD1GU8ZSvS598oqdkZuDFQ
YvejAwIukl62hh3+CFsty4HwyyaMSuNHBMkoC41e2hUVOI2aBXzIQa1ND1OQiDnI
Ef8J1PQWGDrp229guqWMi1tiL2U+U3qsb79S/temJJQx+pFBZFFTMQqdHU0ABJCJ
+jOStlGiG1ecMdH/GOswEXM2qg4HG+MU6f6ttrQ2aJcehGLtXPJVx6S0Ct7HO3MK
6k6vrcO8tthfXeahs5P6ibUe0zr859w/KbmUb+Vf/lYEyimiGZt14x0mj0CNvoJY
uWjKk+5xrodF20DQd29OzO62nhFKYuVCG9Q79VOiWCbNNPPY9265AjwC05LwI++v
oaajq78g1jljB3rWbup3cDRnIhDCBxqTGaC0QGQfGUwtjlPsg6P+u395Nh8ExvSn
syo+4K/mUxYiSimeFmaWvec9S6GBWF5sShKQbCerAdR9UV7BClTBypQ+COTC7rdS
tI32dbmxgnLh9zEycpoemMBcYdvspHZ58cfOdqxuBKrXatHnR/KTs8TCQ7C1J67m
UMSJazRRR1nnWTQ5ngOyYOl9ddY2V1b5Oj2Sjd1J1N9D+ixit8tNuhU2ckRKdeJY
VtQByc4HAgKUKVnQXdKhcRU21advZULT+iFJq0jI3p1BwYRQAvV+9TO7kIt12W/6
5zD1L6nhjVZd0tj+bspH/TgrfAhJfxQdpPH5iY0n8REZRbkj7KC2UMTFOvLyTGDr
hMPv06WJp6PsQsM6ZrhCp9ATJAw12D1BXtVfZ0g4wqkiFmN4pa+0W7hHKpqvQKde
tPx9VA14kyqjVKcFkGM/8xKEhvM/GV4o1p0VSfN7PToUaVwVzD1Zq//2PUaUQCJ2
IAbgyiaeK0dcPBzw5G8zKNz2wiB/v8mEkwPZiMUMKY18/q9Wt8YD4v64Ztfqk9wZ
9RiOTa2KeZ6wEXDAVP3Won2Knwa++WDGkFDs4OYaL/dptT8p8tuSXPFTLQjfU0Dl
CUHCCgl9O/5YRwN9KLEpyY7CRxP0Kf/89Q+60/kZ6SyJEpTjzXd8S1TqCoSWCQdf
pcQf8JARLjBhzw0+FhY6OOwr7BPkltlxai5YFgbwsX9rKUFCRlspLwDktzw7PjNl
bNGH1sBErfYnwEVzjZlb4hjzBp8AgaVEOzcRp/0zTPoI8vTF5ecAF7etwQQp8a/m
FwITxVrEiXpKlzoDG4Rf3qQnV8WrRbYW+p15i/26QxN86NU4UkMJe4TiFkcMiLvT
0xZpmjSc7zePLAWgQRT3mOdDkoav2QA3SVp7Vq1stRMjBaNeifI0nTnOzhwwZzrs
OvJGftuBa7lAcvJJPKQ7jCNFV84HcIfYSiZeveo3PCqHSCbVFCnXyqstxEddXeSE
w2L1gb5AtQ1UEl0OswkiSRIKAJIFaRKtzkgXUC7YXWCwMIflfcvYnJu9ypZ96Y8o
aIqkPwYIP/tbAC/+m/PsVSOmJ6i9ltGcSxeL737BOR4=
EOF
    )"
}

# shellcheck disable=2155
function gcp_on() {
    # Set up everything required to run tests on GCP.
    # Steps from ../tools/provisioning/gcp/README.md have been followed.
    # All sensitive files have been encrypted, see respective functions.
    if [ -z "$SECRET_KEY" ]; then
        echo >&2 "Failed to configure for Google Cloud Platform: no value for the SECRET_KEY environment variable."
        return 1
    fi

    # SSH public key and SSH username:
    export TF_VAR_gcp_public_key_path="$HOME/.ssh/wksctl_cit_id_rsa.pub"
    ssh_public_key >"$TF_VAR_gcp_public_key_path"
    export TF_VAR_gcp_username=$(cut -d' ' -f3 "$TF_VAR_gcp_public_key_path" | cut -d'@' -f1)

    # SSH private key:
    export TF_VAR_gcp_private_key_path=$(set_up_ssh_private_key "$SECRET_KEY")

    # JSON credentials:
    export GOOGLE_CREDENTIALS_FILE="$HOME/.ssh/wksctl-cit.json"
    [ -e "$GOOGLE_CREDENTIALS_FILE" ] && rm -f "$GOOGLE_CREDENTIALS_FILE"
    gcp_credentials "$SECRET_KEY" >"$GOOGLE_CREDENTIALS_FILE"
    chmod 400 "$GOOGLE_CREDENTIALS_FILE"
    export GOOGLE_CREDENTIALS=$(cat "$GOOGLE_CREDENTIALS_FILE")

    export TF_VAR_client_ip=$(curl -s -X GET http://checkip.amazonaws.com/)
    export TF_VAR_gcp_project="${PROJECT:-"weave-net-tests"}"
    # shellcheck disable=2015
    [ -z "$PROJECT" ] && echo >&2 "WARNING: no value provided for PROJECT environment variable: defaulted it to $TF_VAR_gcp_project." || true
}
alias gcp_on='gcp_on'

function gcp_off() {
    unset TF_VAR_gcp_public_key_path
    unset TF_VAR_gcp_username
    unset TF_VAR_gcp_private_key_path
    unset GOOGLE_CREDENTIALS_FILE
    unset GOOGLE_CREDENTIALS
    unset TF_VAR_client_ip
    unset TF_VAR_gcp_project
}
alias gcp_off='gcp_off'

function tf_ssh_usage() {
    cat >&2 <<-EOF
ERROR: $1

Usage:
  \$ tf_ssh <host ID (1-based)> [OPTION]...
Examples:
  \$ tf_ssh 1
  \$ tf_ssh 1 -o LogLevel VERBOSE
  \$ tf_ssh 1 -i ~/.ssh/custom_private_key_id_rsa
Available machines:
EOF
    cat -n >&2 <<<"$(terraform output public_etc_hosts)"
}

# shellcheck disable=SC2155
function tf_ssh() {
    [ -z "$1" ] && tf_ssh_usage "No host ID provided." && return 1
    local ip="$(sed "$1q;d" <<<"$(terraform output public_etc_hosts)" | cut -d ' ' -f 1)"
    shift # Drop the first argument, corresponding to the machine ID, to allow passing other arguments to SSH using "$@" -- see below.
    [ -z "$ip" ] && tf_ssh_usage "Invalid host ID provided." && return 1
    # shellcheck disable=SC2029
    ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no "$@" "$(terraform output username)@$ip"
}
alias tf_ssh='tf_ssh'

function tf_ansi_usage() {
    cat >&2 <<-EOF
ERROR: $1

Usage:
  \$ tf_ansi <playbook or playbook ID (1-based)> [OPTION]...
Examples:
  \$ tf_ansi setup_weave-net_dev
  \$ tf_ansi 1
  \$ tf_ansi 1 -vvv --private-key=~/.ssh/custom_private_key_id_rsa
  \$ tf_ansi setup_weave-kube --extra-vars "docker_version=1.12.6 kubernetes_version=1.5.6"
Available playbooks:
EOF
    cat -n >&2 <<<"$(for file in "$(dirname "${BASH_SOURCE[0]}")"/../../config_management/*.yml; do basename "$file" | sed 's/.yml//'; done)"
}
