output "bootstrap_private_ip" {
  value = data.ibm_pi_instance_ip.bootstrap_ip.ip
}