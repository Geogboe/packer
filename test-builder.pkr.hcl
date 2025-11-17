packer {
  required_plugins {
    null = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/null"
    }
  }
}

source "null" "example" {
  communicator = "none"
}

build {
  sources = ["source.null.example"]

  provisioner "shell-local" {
    inline = [
      "echo 'Build started!'",
      "echo 'Running provisioner 1...'",
      "sleep 1",
      "echo 'Provisioner 1 complete!'",
    ]
  }

  provisioner "shell-local" {
    inline = [
      "echo 'Running provisioner 2...'",
      "sleep 1",
      "echo 'Provisioner 2 complete!'",
    ]
  }

  provisioner "shell-local" {
    inline = [
      "echo 'Running provisioner 3...'",
      "sleep 1",
      "echo 'Provisioner 3 complete!'",
    ]
  }

  post-processor "manifest" {
    output     = "manifest.json"
    strip_path = true
  }
}
