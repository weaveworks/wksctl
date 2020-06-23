#!/bin/bash
#
# Description:
#   This script runs all Weave integration tests on the specified
#   provider (default: Google Cloud Platform).
#
# Usage:
#
#   Run all integration tests on Google Cloud Platform:
#   $ ./run-integration-tests.sh
#
#   Run all integration tests on Amazon Web Services:
#   PROVIDER=aws ./run-integration-tests.sh
#

set -e

export INT_TEST_DIR="$(dirname "$0")/../.."
export REPO_ROOT_DIR="$INT_TEST_DIR/../.."
export PROVISIONING_DIR="$(dirname $0)/provisioning"
. "$PROVISIONING_DIR/setup.sh" # Import gcp_on, do_on, and aws_on.
. "$(dirname $0)/colourise.sh"                      # Import greenly.

# Variables:
export APP="wks"
# shellcheck disable=SC2034
export PROJECT="wks-tests" # Only used when PROVIDER is gcp, by tools/provisioning/config.sh.
export NAME=${NAME:-"$(whoami | sed -e 's/[\.\_]*//g' | cut -c 1-4)"}
export PROVIDER=${PROVIDER:-gcp} # Provision using provided provider, or Google Cloud Platform by default.
export NUM_HOSTS=${NUM_HOSTS:-3}
export PLAYBOOK=${PLAYBOOK:-setup_bare_docker.yml}
export PLAYBOOK_DOCKER_INSTALL_ROLE=${PLAYBOOK_DOCKER_INSTALL_ROLE:-docker-from-get.docker.com}
export TESTS=${TESTS:-}
export RUNNER_ARGS=${RUNNER_ARGS:-""}
# Dependencies' versions:
export DOCKER_VERSION=${DOCKER_VERSION:-"$(grep -oP "(?<=DOCKER_VERSION=).*" "$REPO_ROOT_DIR/DEPENDENCIES")"}
# Google Cloud Platform image's name & usage (only used when PROVIDER is gcp):
export IMAGE_NAME=${IMAGE_NAME:-"$(echo "$APP-centos7-docker$DOCKER_VERSION" | sed -e 's/[\.\_]*//g')"}
export DISK_NAME_PREFIX=${DISK_NAME_PREFIX:-$NAME}
export USE_IMAGE=${USE_IMAGE:-1}
export CREATE_IMAGE=${CREATE_IMAGE:-1}
export CREATE_IMAGE_TIMEOUT_IN_SECS=${CREATE_IMAGE_TIMEOUT_IN_SECS:-600}
# Lifecycle flags:
export SKIP_CONFIG=${SKIP_CONFIG:-}
export SKIP_DESTROY=${SKIP_DESTROY:-}
# Save terraform output for further use in tests
export TERRAFORM_OUTPUT=${TERRAFORM_OUTPUT:-/tmp/terraform_output.json}

function print_vars() {
    echo "--- Variables: Main ---"
    echo "PROVIDER=$PROVIDER"
    echo "NUM_HOSTS=$NUM_HOSTS"
    echo "PLAYBOOK=$PLAYBOOK"
    echo "TESTS=$TESTS"
    echo "SSH_OPTS=$SSH_OPTS"
    echo "RUNNER_ARGS=$RUNNER_ARGS"
    echo "--- Variables: Versions ---"
    echo "DOCKER_VERSION=$DOCKER_VERSION"
    echo "IMAGE_NAME=$IMAGE_NAME"
    echo "DISK_NAME_PREFIX=$DISK_NAME_PREFIX"
    echo "USE_IMAGE=$USE_IMAGE"
    echo "CREATE_IMAGE=$CREATE_IMAGE"
    echo "CREATE_IMAGE_TIMEOUT_IN_SECS=$CREATE_IMAGE_TIMEOUT_IN_SECS"
    echo "--- Variables: Flags ---"
    echo "SKIP_CONFIG=$SKIP_CONFIG"
    echo "SKIP_DESTROY=$SKIP_DESTROY"
    echo "--- Variables: Output ---"
    echo "TERRAFORM_OUTPUT=$TERRAFORM_OUTPUT"
}

function verify_dependencies() {
    local deps=(python terraform gcloud)
    for dep in "${deps[@]}"; do
        if [ ! "$(which "$dep")" ]; then
            echo >&2 "$dep is not installed or not in PATH."
            exit 1
        fi
    done
}

# shellcheck disable=SC2155
function provision_locally() {
    export VAGRANT_CWD="$(dirname "${BASH_SOURCE[0]}")"
    case "$1" in
        on)
            vagrant up
            local status=$?

            # Set up SSH connection details:
            local ssh_config=$(mktemp /tmp/vagrant_ssh_config_XXX)
            vagrant ssh-config >"$ssh_config"
            export SSH="ssh -F $ssh_config"
            # Extract username, SSH private key, and VMs' IP addresses:
            ssh_user="$(sed -ne 's/\ *User //p' "$ssh_config" | uniq)"
            ssh_id_file="$(sed -ne 's/\ *IdentityFile //p' "$ssh_config" | uniq)"
            ssh_hosts=$(sed -ne 's/Host //p' "$ssh_config")

            SKIP_CONFIG=1 # Vagrant directly configures virtual machines using Ansible -- see also: Vagrantfile
            return $status
            ;;
        off)
            vagrant destroy -f
            ;;
        *)
            echo >&2 "Unknown command $1. Usage: {on|off}."
            exit 1
            ;;
    esac
}

function setup_gcloud() {
    # Authenticate:
    gcloud auth activate-service-account --key-file "$GOOGLE_CREDENTIALS_FILE" 1>/dev/null
    # Set current project:
    gcloud config set project $PROJECT
}

function image_exists() {
    greenly echo "> Checking existence of GCP image $IMAGE_NAME..."
    gcloud compute images list | grep "$PROJECT" | grep "$IMAGE_NAME"
}

function image_ready() {
    # GCP images seem to be listed before they are actually ready for use,
    # typically failing the build with: "googleapi: Error 400: The resource is not ready".
    # We therefore consider the image to be ready once the disk of its template instance has been deleted.
    ! gcloud compute disks list | grep "$DISK_NAME_PREFIX"
}

function wait_for_image() {
    greenly echo "> Waiting for GCP image $IMAGE_NAME to be created..."
    for i in $(seq "$CREATE_IMAGE_TIMEOUT_IN_SECS"); do
        image_exists && image_ready && return 0
        if ! ((i % 60)); then echo "Waited for $i seconds and still waiting..."; fi
        sleep 1
    done
    redly echo "> Waited $CREATE_IMAGE_TIMEOUT_IN_SECS seconds for GCP image $IMAGE_NAME to be created, but image could not be found."
    exit 1
}

# shellcheck disable=SC2155
function create_image() {
    if [[ "$CREATE_IMAGE" == 1 ]]; then
        greenly echo "> Creating GCP image $IMAGE_NAME..."
        local begin_img=$(date +%s)
        local num_hosts=1
        terraform apply -input=false -auto-approve -var "app=$APP" -var "gcp_image=centos-cloud/centos-7" -var "name=$NAME" -var "num_hosts=$num_hosts" "$PROVISIONING_DIR/gcp"
        local zone=$(terraform output zone)
        local name=$(terraform output instances_names)
        gcloud -q compute instances delete "$name" --keep-disks boot --zone "$zone"
        gcloud compute images create "$IMAGE_NAME" --source-disk "$name" --source-disk-zone "$zone" \
            --description "Testing image for WKS based on $(terraform output image) and Docker $DOCKER_VERSION."
        gcloud compute disks delete "$name" --zone "$zone"
        terraform destroy -force "$PROVISIONING_DIR/gcp"
        rm terraform.tfstate*
        echo
        local end_img=$(date +%s)
        greenly echo "> Created GCP image $IMAGE_NAME in $(date -u -d @$((end_img - begin_img)) +"%T")."
    else
        wait_for_image
    fi
}

function use_image() {
    setup_gcloud
    export TF_VAR_gcp_image="$IMAGE_NAME" # Override the default image name.
    export SKIP_CONFIG=1                  # No need to configure the image, since already done when making the template
}

# deprecated
#
# Note 1: not sure what the goal of creating images was but it appears to be
# misbehaving due to gcp not playing nicely with slashes in image names
# Note 2: custom images are not using in any integration test path, not sure
# what custom images were for but I'm guessing build caching?
function use_or_create_image() {
    setup_gcloud
    image_exists || create_image
    export TF_VAR_gcp_image="$IMAGE_NAME" # Override the default image name.
    export SKIP_CONFIG=1                  # No need to configure the image, since already done when making the template
}

# shellcheck disable=SC2155
function set_hosts() {
    export HOSTS="$(echo "$ssh_hosts" | tr '\n' ' ')"
}

function terraform_init() {
    terraform init "$PROVISIONING_DIR/$PROVIDER"
}

function provision_remotely() {
    case "$1" in
        on)
            terraform apply -input=false -auto-approve -parallelism="$NUM_HOSTS" -var "app=$APP" -var "name=$NAME" -var "num_hosts=$NUM_HOSTS" "$PROVISIONING_DIR/$2"
            local status=$?
            ssh_user=$(terraform output username)
            ssh_id_file=$(terraform output private_key_path)
            ssh_hosts=$(terraform output hostnames)
            export SSH="ssh -l $ssh_user -i $ssh_id_file $SSH_OPTS"

            terraform output public_etc_hosts  > /tmp/hosts_public
            terraform output private_etc_hosts > /tmp/hosts_private
            greenly echo "Host addresses are in /tmp/hosts_public and /tmp/hosts_private"

            return $status
            ;;
        off)
            terraform destroy -force "$PROVISIONING_DIR/$2"
            ;;
        *)
            echo >&2 "Unknown command $1. Usage: {on|off}."
            exit 1
            ;;
    esac
}

# shellcheck disable=SC2155
function provision() {
    local action=$([ "$1" == "on" ] && echo "Provisioning" || echo "Shutting down")
    local skipped=$([ "$action" == "off" ] && [ -n "$SKIP_DESTROY" ] && echo " (skipped)")
    echo
    greenly echo "> $action test host(s) on [$PROVIDER]...$skipped"

    # Don't destroy when asked not to.
    [ "$1" == "off" ] && [ -n "$SKIP_DESTROY" ] && return

    local begin_prov=$(date +%s)
    case "$2" in
        'aws')
            aws_on
            provision_remotely "$1" "$2"
            ;;
        'do')
            do_on
            provision_remotely "$1" "$2"
            ;;
        'gcp')
            export PATH="$PATH:/opt/google-cloud-sdk/bin"
            export CLOUDSDK_CORE_DISABLE_PROMPTS=1
            gcp_on
            [[ "$1" == "on" ]] && [[ "$USE_IMAGE" == 1 ]] && use_image
            provision_remotely "$1" "$2"
            ;;
        'vagrant')
            provision_locally "$1"
            ;;
        *)
            echo >&2 "Unknown provider $2. Usage: PROVIDER={gcp|aws|do|vagrant}."
            exit 1
            ;;
    esac
    [ "$1" == "on" ] && set_hosts
    echo
    local end_prov=$(date +%s)
    greenly echo "> Provisioning took $(date -u -d @$((end_prov - begin_prov)) +"%T")."
}

# shellcheck disable=SC2155
function configure() {
    echo
    if [ -n "$SKIP_CONFIG" ]; then
        greenly echo "> Skipped configuration of test host(s)."
    else
        greenly echo "> Configuring test host(s)..."
        local begin_conf=$(date +%s)
        # Nothing to do here at present
        local end_conf=$(date +%s)
        greenly echo "> Configuration took $(date -u -d @$((end_conf - begin_conf)) +"%T")."
    fi
}

# shellcheck disable=SC2155
function run_tests() {
    echo
    greenly echo "> Running tests..."
    local begin_tests=$(date +%s)
    set +e # Do not fail this script upon test failure, since we need to shut down the test cluster regardless of success or failure.
    "$INT_TEST_DIR/run_all.sh" "$@"
    local status=$?
    echo
    local end_tests=$(date +%s)
    greenly echo "> Tests took $(date -u -d @$((end_tests - begin_tests)) +"%T")."
    return $status
}

# shellcheck disable=SC2155
function end() {
    echo
    local end_time=$(date +%s)
    echo "> Build took $(date -u -d @$((end_time - begin)) +"%T")."
}

function echo_export_hosts() {
    exec 1>&111
    # Print a command to set HOSTS in the calling script, so that subsequent calls to
    # test scripts can point to the right testing machines while developing:
    echo "export HOSTS=\"$HOSTS\""
    exec 1>&2
}

function export_terraform_output() {
    greenly echo "> Export terraform output to $TERRAFORM_OUTPUT..."
    terraform output -json > $TERRAFORM_OUTPUT
}

function main() {
    # Keep a reference to stdout in another file descriptor (FD #111), and then globally redirect all stdout to stderr.
    # This is so that HOSTS can be eval'ed in the calling script using:
    #   $ eval $(./run-integration-tests.sh [provision|configure|setup])
    # and ultimately subsequent calls to test scripts can point to the right testing machines during development.
    if [ "$1" == "provision" ] || [ "$1" == "configure" ] || [ "$1" == "setup" ]; then
        exec 111>&1 # 111 ought to match the file descriptor used in echo_export_hosts.
        exec 1>&2
    fi

    begin=$(date +%s)
    trap end EXIT

    print_vars
    verify_dependencies

    terraform_init

    case "$1" in
        "") # Provision, configure, run tests, and destroy test environment:
            provision on "$PROVIDER"
            configure "$ssh_user" "$ssh_hosts" "${ssh_port:-22}" "$ssh_id_file"
            "$INT_TEST_DIR/setup.sh"
            run_tests "$TESTS"
            status=$?
            provision off "$PROVIDER"
            exit $status
            ;;

        up) # Setup a test environment without actually doing any testing.
            provision on "$PROVIDER"
            configure "$ssh_user" "$ssh_hosts" "${ssh_port:-22}" "$ssh_id_file"
            "$INT_TEST_DIR/setup.sh"
            export_terraform_output
            echo_export_hosts
            ;;

        provision)
            provision on "$PROVIDER"
            export_terraform_output
            echo_export_hosts
            ;;

        configure)
            provision on "$PROVIDER" # Vagrant and Terraform do not provision twice if VMs are already provisioned, so we just set environment variables.
            configure "$ssh_user" "$ssh_hosts" "${ssh_port:-22}" "$ssh_id_file"
            export_terraform_output
            echo_export_hosts
            ;;

        setup)
            provision on "$PROVIDER" # Vagrant and Terraform do not provision twice if VMs are already provisioned, so we just set environment variables.
            "$INT_TEST_DIR/setup.sh"
            export_terraform_output
            echo_export_hosts
            ;;

        test)
            provision on "$PROVIDER" # Vagrant and Terraform do not provision twice if VMs are already provisioned, so we just set environment variables.
            export_terraform_output
            run_tests "$TESTS"
            ;;

        destroy)
            provision off "$PROVIDER"
            ;;

        *)
            echo "Unknown command: $1" >&2
            exit 1
            ;;
    esac
}

main "$@"
