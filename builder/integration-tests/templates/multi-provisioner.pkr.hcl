packer {
  required_version = ">= 1.7.0"
}

source "null" "multi-provisioner" {
  communicator = "none"
}

build {
  name = "multi-provisioner-test"

  sources = ["source.null.multi-provisioner"]

  # First provisioner - setup
  provisioner "shell-local" {
    inline = [
      "echo '=== Provisioner 1: Setup ==='",
      "mkdir -p /tmp/packer-test-$$",
      "echo 'test data' > /tmp/packer-test-$$/data.txt",
      "sleep 1"
    ]
  }

  # Second provisioner - processing
  provisioner "shell-local" {
    inline = [
      "echo '=== Provisioner 2: Processing ==='",
      "cat /tmp/packer-test-$$/data.txt",
      "echo 'additional data' >> /tmp/packer-test-$$/data.txt",
      "sleep 1"
    ]
  }

  # Third provisioner - validation
  provisioner "shell-local" {
    inline = [
      "echo '=== Provisioner 3: Validation ==='",
      "wc -l /tmp/packer-test-$$/data.txt",
      "cat /tmp/packer-test-$$/data.txt",
      "sleep 1"
    ]
  }

  # Fourth provisioner - cleanup
  provisioner "shell-local" {
    inline = [
      "echo '=== Provisioner 4: Cleanup ==='",
      "rm -rf /tmp/packer-test-$$",
      "echo 'Cleanup complete'"
    ]
  }

  post-processor "manifest" {
    output = "multi-prov-manifest.json"
  }
}
