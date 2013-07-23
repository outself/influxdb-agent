# agent

# Installing

Create an errplane user using the following instruction:

`useradd -r errplane`

To install the package run

`sudo dpkg -i errplane-agent_0.1.0_amd64.deb`

If this is the first time to install the package, edit `/etc/errplane-agent/config.yml`.

An init.d script will be installed to start and stop the agent `/etc/init.d/errplane-agent`
