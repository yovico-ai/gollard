# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure("2") do |config|
    config.vm.box = "bento/ubuntu-24.04"

    config.vm.provider "virtualbox" do |vb|
      vb.gui = false
      vb.cpus = 4
      vb.memory = "8000"
    end

    config.vm.provider "vmware_fusion" do |vb|
      vb.gui = false
      vb.cpus = 7
      vb.memory = "8000"
    end

    config.vm.network "private_network", type: "dhcp"

    config.vm.provision "shell" do |s|
      s.path = "vagrant/provision.sh"
      s.privileged = false
    end
end
