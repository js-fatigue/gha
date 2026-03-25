# No terraform{} block and untyped variable — intentional lint bait

variable "name" {}

output "greeting" {
  value = "Hello, ${var.name}!"
}
