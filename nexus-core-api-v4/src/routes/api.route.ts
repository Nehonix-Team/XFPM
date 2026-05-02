import { Router } from "xypriss";
import { manifest } from "../configs/manifest";
import { xems } from "xypriss";


/**
 * Default API Router
 * 
 * Manages core application business logic.
 */
const router = Router();

/**
 * API Root Endpoint
 * 
 * Responds with application manifest metadata.
 */
router.get("/", (req, res) => {
  return res.success("Hello World from XyPriss!", {
    mode: "default",
    ...manifest,
  });
});

/**
 * XEMS Demo: Create a secure session
 */
router.post("/xems/create", async (req, res) => {
    const runner = xems.forApp(req.app);
    const token = await runner.createSession("demo-sandbox", {
        user: "XyPriss Developer",
        timestamp: Date.now()
    }, { ttl: "30m" });

    res.success({ message: "Session created", token });
});

/**
 * XEMS Demo: Resolve and Rotate
 */
router.post("/xems/resolve", async (req, res) => {
    const runner = xems.forApp(req.app);
    const session = await runner.resolveSession(req.body.token, {
        sandbox: "demo-sandbox",
        rotate: true,
        gracePeriod: 5000
    });

    if (!session) return res.status(401).error("Invalid token");
    res.success({ data: session.data, newToken: session.newToken });
});


export { router as apiRouter };
