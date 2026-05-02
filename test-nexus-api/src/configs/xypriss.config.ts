import { authServer } from "../servers/auth.server";
import { mainServer } from "../servers/main.server";

/**
 * XyPriss Multi-Server (XMS) Orchestration Configuration
 *
 * In XMS mode, security and guardrail settings are applied globally
 * and inherited by all server instances unless overridden in specific
 * server definitions.
 */

export const serverConfigs: ServerOptions = {
  // XMS Orchestration
  multiServer: {
    enabled: true,
    servers: [authServer, mainServer],
    xems: {
      enable: true,
      ttl: "15m",
      autoRotation: false, // Set to true in production for enhanced security (token rotation)
    },
  },


  security: {
    terminalOnly: {
      enable: true,
      allowedTools: ["postman", "curl", "insomnia", "axios"],
    },
  },

  requestManagement: {
    networkQuality: {
      enabled: true,
      rejectOnPoorConnection: true,
      maxLatency: 500,
    },
  },
};

