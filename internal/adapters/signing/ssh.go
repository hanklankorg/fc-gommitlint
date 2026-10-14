// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

// SSH public key algorithm names as reported by ssh.PublicKey.Type().
const (
	sshKeyTypeECDSAP256 = "ecdsa-sha2-nistp256"
	sshKeyTypeECDSAP384 = "ecdsa-sha2-nistp384"
	sshKeyTypeECDSAP521 = "ecdsa-sha2-nistp521"
	sshKeyTypeEd25519   = "ssh-ed25519"
	sshKeyTypeRSA       = "ssh-rsa"
)
