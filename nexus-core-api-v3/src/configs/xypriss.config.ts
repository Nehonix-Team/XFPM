/**
 * XyPriss Default Server Configuration
 *
 * Configured for a standard, single-instance server operation.
 */

import { manifest } from "./manifest";

export const serverConfigs: ServerOptions = {
  /**
   * Server instance configuration.
   * Defines the port and primary identification for this server.
   */
  server: {
    autoKillConflict: true,
    port: __sys__.vars.__PORT__,
    serviceName: manifest.name,
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

