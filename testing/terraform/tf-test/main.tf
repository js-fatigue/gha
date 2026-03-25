terraform {
  required_version = ">= 1.0"
  backend "local" {}
}

output "hello" {
  value = "world"
}
