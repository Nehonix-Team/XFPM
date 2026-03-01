#!/usr/bin/env node
/***************************************************************************
 * xypriss-cli - NPM Compatibility Wrapper
 * This script pulls the native Go-based XFPM installer.
 ***************************************************************************** */

const https = require("https");
const { spawn } = require("child_process");

const INSTALLER_URL =
  "https://raw.githubusercontent.com/Nehonix-Team/XFMP/master/private/install.js";

console.log("\n\x1b[36m[i] Initializing XyPriss Native Gateway...\x1b[0m");
console.log(
  "\x1b[2m[>] Fetching deployment sequence from nehonix.com...\x1b[0m\n",
);

https
  .get(INSTALLER_URL, (res) => {
    if (res.statusCode !== 200) {
      console.error(
        `\x1b[31m[!] Failed to download installer (HTTP ${res.statusCode})\x1b[0m`,
      );
      process.exit(1);
    }

    let script = "";
    res.on("data", (chunk) => {
      script += chunk;
    });
    res.on("end", () => {
      const child = spawn("node", ["-"], {
        stdio: ["pipe", "inherit", "inherit"],
      });

      child.stdin.write(script);
      child.stdin.end();

      child.on("close", (code) => {
        if (code === 0) {
          console.log(
            "\n\x1b[32m[✔] XyPriss Native CLI is now ready.\x1b[0m\n",
          );
        }
        process.exit(code);
      });
    });
  })
  .on("error", (err) => {
    console.error("\x1b[31m[!] Network Error:\x1b[0m", err.message);
    process.exit(1);
  });
