#!/bin/bash
set -e

systemctl daemon-reload
systemctl enable everythingxd
systemctl start everythingxd
