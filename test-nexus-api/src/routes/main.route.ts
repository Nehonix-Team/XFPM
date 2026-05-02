import { Router } from "xypriss";
import { manifest } from "../configs/manifest";

/**
 * Main API Router
 * 
 * Manages core application business logic within the isolated `/api` scope.
 */
const router = Router();

/**
 * API Root Endpoint
 * 
 * Responds with server identity and application manifest metadata for the 
 * main service instance.
 */
router.get("/", (req, res) => {
  return res.success("Hello World from XyPriss!", {
    serverType: "main",
    ...manifest,
  });
});

export { router as mainRouter };
