import { Router } from "xypriss";
import { manifest } from "../configs/manifest";
import { xems } from "xypriss";


/**
 * Authentication Router
 * 
 * Manages identity-related traffic within the isolated `/auth` scope.
 */
const router = Router();

/**
 * Authentication Root Endpoint
 * 
 * Responds with server identity and application manifest metadata.
 */
router.get("/", (req, res) => {
  return res.success("Hello World from XyPriss!", {
    serverType: "auth",
    ...manifest,
  });
});

/**
 * XEMS Auth Demo: Create a secure session
 */
router.post("/xems/create", async (req, res) => {
    const runner = xems.forApp(req.app);
    const token = await runner.createSession("auth-sandbox", {
        userId: "xy-123",
        role: "admin"
    }, { ttl: "30m" });

    res.success({ message: "Auth session created", token });
});

/**
 * XEMS Auth Demo: Resolve and Rotate
 */
router.post("/xems/resolve", async (req, res) => {
    const runner = xems.forApp(req.app);
    const session = await runner.resolveSession(req.body.token, {
        sandbox: "auth-sandbox",
        rotate: true,
        gracePeriod: 5000
    });

    if (!session) return res.status(401).error("Invalid auth token");
    res.success({ data: session.data, newToken: session.newToken });
});


export { router as authRouter };
