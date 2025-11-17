packer {
  required_version = ">= 1.7.0"
}

source "null" "basic" {
  communicator = "none"
}

build {
  name = "basic-null-build"

  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo 'Running basic null build test'",
      "echo 'Build started at: '$(date)",
      "sleep 2",
      "echo 'Build completed successfully'"
    ]
  }

  post-processor "manifest" {
    output = "manifest.json"
  }
}
