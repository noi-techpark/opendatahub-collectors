# SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
# SPDX-License-Identifier: CC0-1.0
#
# based on https://github.com/atmoz/sftp 
# SPDX-FileCopyrightText: 2015-2025 Adrian Dvergsdal
# SPDX-License-Identifier: MIT

set -Eeo pipefail

# create user
user=$SFTP_USER
pass=$SFTP_PASS

useradd --no-user-group "$user"
echo "$user:$pass" | chpasswd
uid="$(id -u "$user")"

# create user directories
mkdir -p "/home/sftp/upload"
chown root:root "/home/sftp"
chmod 755 "/home/sftp"
chown "$uid":users "/home/sftp/upload"

chmod 600 /etc/ssh/ssh_host_ed25519_key || true
chmod 600 /etc/ssh/ssh_host_rsa_key || true

# write sshd_config
cat << EOF > /etc/sshd_config
Protocol 2
HostKey /etc/ssh/ssh_host_ed25519_key
HostKey /etc/ssh/ssh_host_rsa_key
UseDNS no
PermitRootLogin no
X11Forwarding no
AllowTcpForwarding no
Subsystem sftp internal-sftp
ForceCommand internal-sftp
ChrootDirectory %h
HostKeyAlgorithms +ssh-rsa
EOF

# start SSHD daemon
exec /usr/sbin/sshd -e 

