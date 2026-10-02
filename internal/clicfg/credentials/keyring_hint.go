// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import "runtime"

func KeyringSetupHint() string {
	switch runtime.GOOS {
	case "linux":
		return "To set up the keyring daemon, run:\n" +
			"  sudo apt-get install gnome-keyring libsecret-1-0\n" +
			"  gnome-keyring-daemon --start --components=secrets"
	case "darwin":
		return "To unlock the Keychain, run:\n" +
			"  security unlock-keychain ~/Library/Keychains/login.keychain-db"
	case "windows":
		return "Ensure the Windows Credential Manager service is running.\n" +
			"Check its status with: Get-Service VaultSvc\n" +
			"Or open services.msc and start the 'Credential Manager' service."
	default:
		return "Ensure your system keyring daemon is running."
	}
}
