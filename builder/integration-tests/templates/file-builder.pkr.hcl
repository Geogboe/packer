packer {
  required_version = ">= 1.7.0"
}

source "file" "test" {
  content = "Hello from Packer Fork Builder!\nThis is a test file.\nTimestamp: ${timestamp()}\n"
  target  = "output/test-file.txt"
}

build {
  name = "file-builder-test"

  sources = ["source.file.test"]

  provisioner "shell-local" {
    inline = [
      "echo 'File builder test starting'",
      "test -f output/test-file.txt && echo 'File created successfully' || echo 'File creation failed'",
      "cat output/test-file.txt",
      "echo 'Test completed'"
    ]
  }

  post-processor "manifest" {
    output = "file-manifest.json"
  }
}
