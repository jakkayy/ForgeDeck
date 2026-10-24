"""Browser benchmark: cold-cache largest-contentful-paint of the main dashboard pages.

Run it against a Control Plane that holds scripts/bench/seed.sql data, as bench@forgedeck.local:

    python3 scripts/bench/browser.py --url http://127.0.0.1:8080 --password '<password>'

Needs `pip install playwright` and `playwright install chromium`. Profiles: "desktop" (no throttling)
and "mobile" (390 px, 4x CPU slowdown, 1.6 Mbit/s down, 150 ms round trip), as in docs/12-measurements.md.
"""

import argparse
import json
import statistics
import sys

from playwright.sync_api import sync_playwright

PROJECT = "00000000-0000-4000-8000-000000000001"
APPLICATION = "00000000-0000-4000-8003-000000000001"
ENVIRONMENT = "00000000-0000-4000-8004-000000000001"

PROFILES = {
    "desktop": {"viewport": {"width": 1280, "height": 800}, "cpu": 1, "network": None},
    "mobile": {
        "viewport": {"width": 390, "height": 800},
        "cpu": 4,
        "network": {"latency": 150, "down": 1.6e6 / 8, "up": 0.75e6 / 8},
    },
}

LCP_OBSERVER = """
window.__lcp = 0;
new PerformanceObserver((list) => {
  for (const e of list.getEntries()) window.__lcp = e.startTime;
}).observe({ type: 'largest-contentful-paint', buffered: true });
"""


def median(values):
    return round(statistics.median(values))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:8080")
    parser.add_argument("--email", default="bench@forgedeck.local")
    parser.add_argument("--password", required=True)
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--profile", choices=PROFILES, default="mobile")
    parser.add_argument("--json", help="write the results to this file")
    args = parser.parse_args()
    profile = PROFILES[args.profile]

    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        setup = browser.new_context(viewport=profile["viewport"])
        page = setup.new_page()
        page.goto(args.url + "/login")
        page.fill('input[name="email"]', args.email)
        page.fill('input[name="password"]', args.password)
        page.click('button[type="submit"]')
        page.wait_for_url(args.url + "/")
        newest_job = page.request.get(args.url + "/api/v1/jobs").json()["items"][0]["id"]
        state = setup.storage_state()
        setup.close()

        pages = {
            "Overview": "/",
            "Servers": "/servers",
            "Projects": "/projects",
            "Jobs": "/jobs",
            "Project": f"/projects/{PROJECT}",
            "Application": f"/applications/{APPLICATION}",
            "Environment": f"/environments/{ENVIRONMENT}",
            "Job log": f"/jobs/{newest_job}",
        }
        results = {}
        for name, path in pages.items():
            lcps, transferred, fonts = [], [], []
            for _ in range(args.runs):
                context = browser.new_context(viewport=profile["viewport"], storage_state=state)
                context.add_init_script(LCP_OBSERVER)
                tab = context.new_page()
                sizes = {"bytes": 0, "fonts": 0}
                urls = {}
                cdp = context.new_cdp_session(tab)
                cdp.send("Network.enable")
                cdp.on("Network.requestWillBeSent", lambda e, urls=urls: urls.update({e["requestId"]: e["request"]["url"]}))

                def on_finished(event, sizes=sizes, urls=urls):
                    size = int(event.get("encodedDataLength", 0))
                    sizes["bytes"] += size
                    if urls.get(event["requestId"], "").endswith(".woff2"):
                        sizes["fonts"] += size

                cdp.on("Network.loadingFinished", on_finished)
                if profile["network"]:
                    net = profile["network"]
                    cdp.send(
                        "Network.emulateNetworkConditions",
                        {
                            "offline": False,
                            "latency": net["latency"],
                            "downloadThroughput": net["down"],
                            "uploadThroughput": net["up"],
                        },
                    )
                if profile["cpu"] > 1:
                    cdp.send("Emulation.setCPUThrottlingRate", {"rate": profile["cpu"]})
                tab.goto(args.url + path, wait_until="networkidle")
                tab.wait_for_timeout(500)
                lcps.append(tab.evaluate("window.__lcp"))
                transferred.append(sizes["bytes"])
                fonts.append(sizes["fonts"])
                context.close()
            results[name] = {
                "lcp_ms": median(lcps),
                "runs": [round(v) for v in lcps],
                "kb": round(statistics.median(transferred) / 1024),
                "font_kb": round(statistics.median(fonts) / 1024),
            }
            print(f"{name:12} LCP {results[name]['lcp_ms']:>5} ms  {results[name]['kb']:>4} KB  fonts {results[name]['font_kb']:>3} KB  runs {results[name]['runs']}")
        browser.close()

    if args.json:
        with open(args.json, "w") as out:
            json.dump(results, out, indent=2)
    return 0


if __name__ == "__main__":
    sys.exit(main())
