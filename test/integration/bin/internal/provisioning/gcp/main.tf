provider "google" {
  version = "~> 1.19.0"

  # Set the below environment variables:
  # - GOOGLE_CREDENTIALS
  # - GOOGLE_PROJECT
  # - GOOGLE_REGION
  # or configure directly below.
  # See also:
  # - https://www.terraform.io/docs/providers/google/
  # - https://console.cloud.google.com/apis/credentials/serviceaccountkey?project=<PROJECT ID>&authuser=1
  region = "${var.gcp_region}"

  project = "${var.gcp_project}"
}

resource "google_compute_instance" "tf_test_vm" {
  name         = "${var.name}-${count.index}"
  machine_type = "${var.gcp_size}"
  zone         = "${var.gcp_zone}"
  count        = "${var.num_hosts}"

  boot_disk {
    initialize_params {
        image = "${var.gcp_image}"
    }
  }

  tags = [
    "${var.app}",
    "${var.name}",
    "terraform",
  ]

  network_interface {
    network = "${var.gcp_network}"

    access_config {
      // Ephemeral IP
    }
  }

  metadata {
    ssh-keys = "${var.gcp_username}:${file("${var.gcp_public_key_path}")}"
  }

  # self destruct after 48h hours to cut down costs
  # this is a fallback in case preemptible is disabled or doesn't occur
  # reference: https://cloud.google.com/community/tutorials/create-a-self-deleting-virtual-machine
  metadata_startup_script = <<-EOT
  #!/bin/sh
  sleep 48h
  export NAME=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/name -H 'Metadata-Flavor: Google')
  export ZONE=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/zone -H 'Metadata-Flavor: Google')
  gcloud --quiet compute instances delete $NAME --zone=$ZONE
  EOT

  # make the VM preemptible to cut down costs
  # and prevent VM from being restarted after preemption
  scheduling {
    preemptible = true
    automatic_restart = false
  }

  # Wait for machine to be SSH-able:
  provisioner "remote-exec" {
    inline = ["exit"]

    connection {
      type        = "ssh"
      user        = "${var.gcp_username}"
      private_key = "${file("${var.gcp_private_key_path}")}"
    }
  }

  timeouts {
    delete = "10m"
  }
}

resource "google_compute_firewall" "fw-allow-docker-and-weave" {
  name        = "${var.name}-allow-docker-and-weave"
  network     = "${var.gcp_network}"
  target_tags = ["${var.name}"]

  allow {
    protocol = "tcp"
    ports    = ["2375", "12375"]
  }

  source_ranges = ["${var.client_ip}"]
}

# Required for FastDP crypto in Weave Net:
resource "google_compute_firewall" "fw-allow-esp" {
  name        = "${var.name}-allow-esp"
  network     = "${var.gcp_network}"
  target_tags = ["${var.name}"]

  allow {
    protocol = "esp"
  }

  source_ranges = ["${var.gcp_network_global_cidr}"]
}

# Required for WKS Kubernetes API server access
resource "google_compute_firewall" "fw-allow-kube-apiserver" {
  name        = "${var.name}-allow-kube-apiserver"
  network     = "${var.gcp_network}"
  target_tags = ["${var.name}"]

  allow {
    protocol = "tcp"
    ports    = ["6443"]
  }

  source_ranges = ["${var.client_ip}"]
}
