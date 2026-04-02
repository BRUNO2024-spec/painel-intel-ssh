#!/bin/bash
# Monitor de login/logout SSH para integração com o painel
LOG_FILE="/var/log/ssh-monitor.log"
echo "$(date) USER=$PAM_USER IP=$PAM_RHOST EVENT=$PAM_TYPE" >> "$LOG_FILE"
