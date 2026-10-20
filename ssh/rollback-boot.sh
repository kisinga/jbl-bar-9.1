#!/bin/sh
# Disable automatic SSH startup. Current SSH sessions and files remain.
exec /bin/sh /var/JBLLocalSSH/hooks.sh rollback
