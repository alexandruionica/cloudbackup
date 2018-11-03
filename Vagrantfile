Vagrant.configure("2") do |config|
  config.vm.box = "windows2016"
  config.vm.box_check_update = false

  # Settings specific to a windows box
  config.vm.guest = :windows
  config.vm.communicator = :winrm
  config.winrm.username = "vagrant"
  config.winrm.password = "vagrant"

  config.ssh.insert_key = false

  config.vm.synced_folder "../..", "/Users/vagrant/Documents/golang"

  config.vm.provider "virtualbox" do |vb|
    # Display the VirtualBox GUI when booting the machine
    vb.gui = true

     # Customize the amount of memory on the VM:
     vb.memory = "2048"
  end

  config.vm.provision "shell", inline: 'setx GOPATH %USERPROFILE%\Documents\golang\ '

end
