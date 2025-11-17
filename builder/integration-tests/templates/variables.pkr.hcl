packer {
  required_version = ">= 1.7.0"
}

variable "test_value" {
  type    = string
  default = "default-value"
}

source "null" "variable-test" {
  communicator = "none"
}

build {
  name = "variable-interpolation-test"

  sources = ["source.null.variable-test"]

  provisioner "shell-local" {
    environment_vars = [
      "TEST_VAR=${var.test_value}",
      "BUILD_NAME=${build.name}",
    ]
    inline = [
      "echo 'Testing variable interpolation'",
      "echo 'TEST_VAR='$TEST_VAR",
      "echo 'BUILD_NAME='$BUILD_NAME",
      "echo 'PackerRunUUID='$PACKER_RUN_UUID",
    ]
  }
}
