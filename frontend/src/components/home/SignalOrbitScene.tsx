"use client";

import { useEffect, useRef } from "react";
import Image from "next/image";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";

if (typeof window !== "undefined") {
  gsap.registerPlugin(useGSAP);
}

const SIGNAL_COLORS = [0x3f3f3f, 0x666666, 0x858585, 0xb8b8b8] as const;

type SignalOrbitSceneProps = {
  className?: string;
};

type NavigatorWithPerformanceHints = Navigator & {
  connection?: {
    effectiveType?: string;
    saveData?: boolean;
  };
  deviceMemory?: number;
};

function shouldUseStaticFallback(motionQuery: MediaQueryList) {
  const browser = navigator as NavigatorWithPerformanceHints;
  const compactViewport = window.matchMedia("(max-width: 767px)").matches;
  const limitedMemory =
    typeof browser.deviceMemory === "number" && browser.deviceMemory <= 4;
  const limitedCpu =
    typeof browser.hardwareConcurrency === "number" &&
    browser.hardwareConcurrency <= 4;
  const slowConnection = ["slow-2g", "2g"].includes(
    browser.connection?.effectiveType ?? ""
  );

  return (
    motionQuery.matches ||
    browser.connection?.saveData === true ||
    slowConnection ||
    limitedMemory ||
    (compactViewport && limitedCpu)
  );
}

function supportsWebGL2() {
  if (typeof window.WebGL2RenderingContext === "undefined") return false;

  let context: WebGL2RenderingContext | null = null;
  try {
    const probe = document.createElement("canvas");
    context = probe.getContext("webgl2", {
      alpha: true,
      antialias: false,
      failIfMajorPerformanceCaveat: true,
    });
    return context !== null;
  } catch {
    return false;
  } finally {
    try {
      context?.getExtension("WEBGL_lose_context")?.loseContext();
    } catch {
      // A probe context may disappear before its explicit release.
    }
  }
}

/**
 * A progressively enhanced signal map: the shipped radar artwork remains the
 * fallback while Three.js is loaded and whenever WebGL is unavailable.
 */
export function SignalOrbitScene({ className = "" }: SignalOrbitSceneProps) {
  const figureRef = useRef<HTMLElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const fallbackRef = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const media = gsap.matchMedia();

      media.add(
        {
          reduceMotion: "(prefers-reduced-motion: reduce)",
          allowMotion: "(prefers-reduced-motion: no-preference)",
        },
        (context) => {
          const reduceMotion = Boolean(context.conditions?.reduceMotion);

          if (reduceMotion) {
            gsap.set(".signal-orbit-label", {
              autoAlpha: 1,
              x: 0,
              y: 0,
              scale: 1,
            });
            return;
          }

          const intro = gsap.timeline({
            defaults: { duration: 0.7, ease: "power3.out" },
          });

          intro
            .fromTo(
              ".signal-orbit-status",
              { autoAlpha: 0, y: 12, scale: 0.96 },
              { autoAlpha: 1, y: 0, scale: 1 }
            )
            .fromTo(
              ".signal-orbit-source",
              { autoAlpha: 0, x: -10, y: 8 },
              {
                autoAlpha: 1,
                x: 0,
                y: 0,
                stagger: { each: 0.09, from: "start" },
              },
              "-=0.38"
            )
            .fromTo(
              ".signal-orbit-heat",
              { autoAlpha: 0, x: 14, scale: 0.96 },
              { autoAlpha: 1, x: 0, scale: 1, duration: 0.8 },
              "-=0.45"
            );
        }
      );

      return () => media.revert();
    },
    { scope: figureRef }
  );

  useEffect(() => {
    const canvas = canvasRef.current;
    const figure = figureRef.current;
    const fallback = fallbackRef.current;

    if (!canvas || !figure || !fallback) return;

    let disposed = false;
    let disposeScene: (() => void) | undefined;

    const showFallback = () => {
      canvas.style.opacity = "0";
      fallback.style.opacity = "1";
      figure.dataset.webgl = "fallback";
    };

    const showScene = () => {
      canvas.style.opacity = "1";
      fallback.style.opacity = "0";
      figure.dataset.webgl = "ready";
    };

    showFallback();

    const motionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    if (shouldUseStaticFallback(motionQuery) || !supportsWebGL2()) {
      return () => {
        disposed = true;
      };
    }

    void import("three")
      .then((THREE) => {
        if (disposed || motionQuery.matches) return;

        const createRenderer = () =>
          new THREE.WebGLRenderer({
            alpha: true,
            antialias: true,
            canvas,
          });

        let renderer: ReturnType<typeof createRenderer>;
        try {
          renderer = createRenderer();
        } catch {
          showFallback();
          return;
        }

        const geometries: Array<{ dispose: () => void }> = [];
        const materials: Array<{ dispose: () => void }> = [];
        const cleanups: Array<() => void> = [];
        let scene: InstanceType<typeof THREE.Scene> | undefined;
        let rafId: number | null = null;
        let elapsed = 0;
        let previousTime = 0;
        let contextLost = false;
        let inViewport = true;
        let sceneDisposed = false;

        const teardown = () => {
          if (sceneDisposed) return;
          sceneDisposed = true;

          if (rafId !== null) {
            window.cancelAnimationFrame(rafId);
            rafId = null;
          }
          previousTime = 0;

          for (const cleanup of cleanups.splice(0).reverse()) {
            try {
              cleanup();
            } catch {
              // Continue releasing the remaining GPU and browser resources.
            }
          }

          try {
            scene?.clear();
          } catch {
            // Continue disposal even when scene detachment fails.
          }
          for (const geometry of geometries) {
            try {
              geometry.dispose();
            } catch {
              // A failed resource must not prevent the rest from being released.
            }
          }
          for (const material of materials) {
            try {
              material.dispose();
            } catch {
              // A failed resource must not prevent the rest from being released.
            }
          }
          try {
            renderer.setAnimationLoop(null);
            renderer.dispose();
          } catch {
            // Continue to force-release a partially disposed renderer.
          } finally {
            try {
              renderer.forceContextLoss();
            } catch {
              // The context may already be unavailable after a device-level loss.
            }
          }
          if (!disposed) showFallback();
        };

        // Register teardown before scene construction so every later failure is safe.
        disposeScene = teardown;

        function keepGeometry<T extends { dispose: () => void }>(geometry: T) {
          geometries.push(geometry);
          return geometry;
        }

        function keepMaterial<T extends { dispose: () => void }>(material: T) {
          materials.push(material);
          return material;
        }

        try {
          scene = new THREE.Scene();
          const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 40);
          camera.position.set(0, 0.15, 8.2);

          renderer.setClearColor(0x000000, 0);
          renderer.outputColorSpace = THREE.SRGBColorSpace;

          const field = new THREE.Group();
          field.rotation.set(-0.12, 0.08, -0.08);
          scene.add(field);

          scene.add(new THREE.AmbientLight(0xffffff, 0.65));
          const keyLight = new THREE.PointLight(0xffffff, 7.5, 18, 2);
          keyLight.position.set(2.5, 3.5, 5);
          scene.add(keyLight);
          const fillLight = new THREE.PointLight(0x8a8a8a, 4, 14, 2);
          fillLight.position.set(-3, -2, 3);
          scene.add(fillLight);

          // Deterministic noise keeps hydration snapshots and visual output stable.
          let seed = 0x5f3759df;
          const random = () => {
            seed = (seed * 1664525 + 1013904223) >>> 0;
            return seed / 0x100000000;
          };

          const particleCount = 260;
          const positions = new Float32Array(particleCount * 3);
          const particleColors = new Float32Array(particleCount * 3);

          for (let index = 0; index < particleCount; index += 1) {
            const angle = random() * Math.PI * 2;
            const radius = 0.85 + Math.pow(random(), 0.62) * 3.35;
            const offset = index * 3;
            const color = new THREE.Color(
              SIGNAL_COLORS[index % SIGNAL_COLORS.length]
            );

            positions[offset] = Math.cos(angle) * radius;
            positions[offset + 1] = Math.sin(angle) * radius * 0.64;
            positions[offset + 2] = (random() - 0.5) * 1.65;
            particleColors[offset] = color.r;
            particleColors[offset + 1] = color.g;
            particleColors[offset + 2] = color.b;
          }

          const particleGeometry = keepGeometry(new THREE.BufferGeometry());
          particleGeometry.setAttribute(
            "position",
            new THREE.BufferAttribute(positions, 3)
          );
          particleGeometry.setAttribute(
            "color",
            new THREE.BufferAttribute(particleColors, 3)
          );
          const particleMaterial = keepMaterial(
            new THREE.PointsMaterial({
              depthWrite: false,
              opacity: 0.38,
              size: 0.047,
              sizeAttenuation: true,
              transparent: true,
              vertexColors: true,
            })
          );
          const particles = new THREE.Points(
            particleGeometry,
            particleMaterial
          );
          field.add(particles);

          const orbitConfigs = [
            {
              color: SIGNAL_COLORS[0],
              rx: 3.45,
              ry: 1.42,
              speed: 0.22,
              tilt: 0.08,
            },
            {
              color: SIGNAL_COLORS[1],
              rx: 2.85,
              ry: 2.02,
              speed: -0.17,
              tilt: -0.34,
            },
            {
              color: SIGNAL_COLORS[2],
              rx: 2.18,
              ry: 1.06,
              speed: 0.29,
              tilt: 0.52,
            },
            {
              color: SIGNAL_COLORS[3],
              rx: 3.72,
              ry: 1.76,
              speed: -0.13,
              tilt: 0.88,
            },
          ];

          const createSignalNode = (color: number) => {
            const geometry = keepGeometry(
              new THREE.SphereGeometry(0.085, 16, 16)
            );
            const material = keepMaterial(
              new THREE.MeshBasicMaterial({
                color,
                opacity: 0.72,
                toneMapped: false,
                transparent: true,
              })
            );
            const node = new THREE.Mesh(geometry, material);

            const haloGeometry = keepGeometry(
              new THREE.RingGeometry(0.13, 0.2, 28)
            );
            const haloMaterial = keepMaterial(
              new THREE.MeshBasicMaterial({
                color,
                depthWrite: false,
                opacity: 0.18,
                side: THREE.DoubleSide,
                transparent: true,
              })
            );
            const halo = new THREE.Mesh(haloGeometry, haloMaterial);
            halo.position.z = -0.01;
            node.add(halo);
            return node;
          };

          const orbitSignals: Array<{
            phase: number;
            rx: number;
            ry: number;
            speed: number;
            tilt: number;
            node: ReturnType<typeof createSignalNode>;
          }> = [];

          orbitConfigs.forEach((config, orbitIndex) => {
            const orbitPositions: number[] = [];
            const segments = 180;

            for (let segment = 0; segment < segments; segment += 1) {
              const angle = (segment / segments) * Math.PI * 2;
              orbitPositions.push(
                Math.cos(angle) * config.rx,
                Math.sin(angle) * config.ry,
                Math.sin(angle * 2 + config.tilt) * 0.2
              );
            }

            const orbitGeometry = keepGeometry(new THREE.BufferGeometry());
            orbitGeometry.setAttribute(
              "position",
              new THREE.Float32BufferAttribute(orbitPositions, 3)
            );
            const orbitMaterial = keepMaterial(
              new THREE.LineBasicMaterial({
                color: config.color,
                opacity: orbitIndex === 1 ? 0.24 : 0.16,
                transparent: true,
              })
            );
            const orbit = new THREE.LineLoop(orbitGeometry, orbitMaterial);
            orbit.rotation.z = config.tilt;
            field.add(orbit);

            for (let signalIndex = 0; signalIndex < 2; signalIndex += 1) {
              const node = createSignalNode(config.color);
              field.add(node);
              orbitSignals.push({
                node,
                phase: orbitIndex * 0.72 + signalIndex * Math.PI,
                rx: config.rx,
                ry: config.ry,
                speed: config.speed * (signalIndex === 0 ? 1 : 0.76),
                tilt: config.tilt,
              });
            }
          });

          const coreGeometry = keepGeometry(
            new THREE.IcosahedronGeometry(0.48, 2)
          );
          const coreMaterial = keepMaterial(
            new THREE.MeshStandardMaterial({
              color: 0x858585,
              emissive: 0xb8b8b8,
              emissiveIntensity: 0.12,
              metalness: 0.55,
              roughness: 0.4,
            })
          );
          const core = new THREE.Mesh(coreGeometry, coreMaterial);
          field.add(core);

          const coreRings = [
            {
              color: SIGNAL_COLORS[0],
              radius: 0.74,
              tube: 0.014,
              rotationX: 1.22,
            },
            {
              color: SIGNAL_COLORS[3],
              radius: 0.92,
              tube: 0.01,
              rotationX: 0.64,
            },
          ].map((ring) => {
            const geometry = keepGeometry(
              new THREE.TorusGeometry(ring.radius, ring.tube, 8, 96)
            );
            const material = keepMaterial(
              new THREE.MeshBasicMaterial({
                color: ring.color,
                opacity: 0.28,
                transparent: true,
              })
            );
            const mesh = new THREE.Mesh(geometry, material);
            mesh.rotation.x = ring.rotationX;
            field.add(mesh);
            return mesh;
          });

          const positionSignals = (time: number) => {
            for (const signal of orbitSignals) {
              const angle = signal.phase + time * signal.speed;
              const x = Math.cos(angle) * signal.rx;
              const y = Math.sin(angle) * signal.ry;
              signal.node.position.set(
                x * Math.cos(signal.tilt) - y * Math.sin(signal.tilt),
                x * Math.sin(signal.tilt) + y * Math.cos(signal.tilt),
                Math.sin(angle * 2 + signal.tilt) * 0.2 + 0.08
              );
            }
          };

          const renderStaticFrame = () => {
            if (sceneDisposed || contextLost || !scene) return false;
            positionSignals(elapsed);
            try {
              renderer.render(scene, camera);
              return true;
            } catch {
              teardown();
              return false;
            }
          };

          const stop = () => {
            if (rafId !== null) {
              window.cancelAnimationFrame(rafId);
              rafId = null;
            }
            previousTime = 0;
          };

          const frame = (time: number) => {
            rafId = null;
            if (
              disposed ||
              sceneDisposed ||
              contextLost ||
              !inViewport ||
              document.hidden ||
              motionQuery.matches
            ) {
              return;
            }

            const delta = previousTime
              ? Math.min((time - previousTime) / 1000, 0.05)
              : 0;
            previousTime = time;
            elapsed += delta;

            particles.rotation.z = elapsed * 0.025;
            particles.rotation.y = Math.sin(elapsed * 0.18) * 0.08;
            core.rotation.x = elapsed * 0.22;
            core.rotation.y = elapsed * 0.32;
            coreRings[0].rotation.z = elapsed * 0.26;
            coreRings[1].rotation.z = -elapsed * 0.19;
            if (renderStaticFrame() && !sceneDisposed) {
              rafId = window.requestAnimationFrame(frame);
            }
          };

          const start = () => {
            if (
              rafId !== null ||
              disposed ||
              sceneDisposed ||
              contextLost ||
              !inViewport ||
              document.hidden ||
              motionQuery.matches
            ) {
              return;
            }
            rafId = window.requestAnimationFrame(frame);
          };

          const resize = () => {
            if (disposed || sceneDisposed || contextLost) return;
            try {
              const bounds = figure.getBoundingClientRect();
              const width = Math.max(1, Math.round(bounds.width));
              const height = Math.max(1, Math.round(bounds.height));

              renderer.setPixelRatio(
                Math.min(window.devicePixelRatio || 1, 1.8)
              );
              renderer.setSize(width, height, false);
              camera.aspect = width / height;
              camera.position.z = width < 520 ? 9.4 : 8.2;
              camera.updateProjectionMatrix();
              renderStaticFrame();
            } catch {
              teardown();
            }
          };

          const handleVisibilityChange = () => {
            if (document.hidden) stop();
            else if (motionQuery.matches) renderStaticFrame();
            else start();
          };

          const handleMotionChange = () => {
            if (motionQuery.matches) {
              stop();
              renderStaticFrame();
            } else {
              start();
            }
          };

          const handleIntersection: IntersectionObserverCallback = (
            entries
          ) => {
            const entry = entries[0];
            if (!entry) return;
            inViewport = entry.isIntersecting && entry.intersectionRatio > 0;
            if (!inViewport) stop();
            else if (motionQuery.matches) renderStaticFrame();
            else start();
          };

          const handleContextLost = (event: Event) => {
            event.preventDefault();
            contextLost = true;
            stop();
            showFallback();
          };

          const handleContextRestored = () => {
            if (disposed || sceneDisposed) return;
            contextLost = false;
            resize();
            if (sceneDisposed) return;
            showScene();
            start();
          };

          if (typeof ResizeObserver === "undefined") {
            window.addEventListener("resize", resize, { passive: true });
            cleanups.push(() => window.removeEventListener("resize", resize));
          } else {
            const resizeObserver = new ResizeObserver(resize);
            cleanups.push(() => resizeObserver.disconnect());
            resizeObserver.observe(figure);
          }

          if (typeof IntersectionObserver !== "undefined") {
            const intersectionObserver = new IntersectionObserver(
              handleIntersection,
              { rootMargin: "120px 0px", threshold: 0 }
            );
            cleanups.push(() => intersectionObserver.disconnect());
            intersectionObserver.observe(figure);
          }

          document.addEventListener("visibilitychange", handleVisibilityChange);
          cleanups.push(() =>
            document.removeEventListener(
              "visibilitychange",
              handleVisibilityChange
            )
          );

          if (typeof motionQuery.addEventListener === "function") {
            motionQuery.addEventListener("change", handleMotionChange);
            cleanups.push(() =>
              motionQuery.removeEventListener("change", handleMotionChange)
            );
          } else {
            motionQuery.addListener(handleMotionChange);
            cleanups.push(() => motionQuery.removeListener(handleMotionChange));
          }

          canvas.addEventListener("webglcontextlost", handleContextLost);
          cleanups.push(() =>
            canvas.removeEventListener("webglcontextlost", handleContextLost)
          );
          canvas.addEventListener(
            "webglcontextrestored",
            handleContextRestored
          );
          cleanups.push(() =>
            canvas.removeEventListener(
              "webglcontextrestored",
              handleContextRestored
            )
          );

          resize();
          if (!sceneDisposed) {
            showScene();
            start();
          }
        } catch {
          teardown();
        }
      })
      .catch(() => {
        if (!disposed) showFallback();
      });

    return () => {
      disposed = true;
      disposeScene?.();
    };
  }, []);

  return (
    <figure
      ref={figureRef}
      role="img"
      aria-label="多来源信号汇聚成热点事件的动态轨迹"
      data-animation="gsap-three"
      data-webgl="loading"
      className={`relative isolate min-h-[360px] w-full overflow-hidden rounded-2xl bg-[#f2f2f2] [box-shadow:var(--shadow-card)] sm:min-h-[460px] dark:bg-[#171717] ${className}`}
    >
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[radial-gradient(circle_at_50%_42%,rgba(0,0,0,0.055),transparent_34%),radial-gradient(circle_at_14%_18%,rgba(0,0,0,0.03),transparent_30%),radial-gradient(circle_at_86%_82%,rgba(0,0,0,0.025),transparent_32%)] dark:bg-[radial-gradient(circle_at_50%_42%,rgba(255,255,255,0.065),transparent_34%),radial-gradient(circle_at_14%_18%,rgba(255,255,255,0.035),transparent_30%),radial-gradient(circle_at_86%_82%,rgba(255,255,255,0.025),transparent_32%)]"
      />
      <div
        ref={fallbackRef}
        aria-hidden="true"
        className="pointer-events-none absolute inset-[8%] opacity-100 grayscale mix-blend-multiply transition-opacity duration-500 dark:invert dark:mix-blend-screen"
      >
        <Image
          src="/images/hotkey-signal-radar.png"
          alt=""
          aria-hidden="true"
          fill
          preload
          sizes="(max-width: 640px) 92vw, (max-width: 1024px) 72vw, 46vw"
          draggable={false}
          className="object-contain opacity-[0.42]"
        />
      </div>
      <canvas
        ref={canvasRef}
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 h-full w-full opacity-0 transition-opacity duration-500"
      />

      <div className="signal-orbit-label signal-orbit-status pointer-events-none absolute left-5 top-5 flex items-center gap-2 rounded-full bg-white/75 px-3 py-2 text-xs font-semibold tracking-[0.08em] text-[#525252] backdrop-blur-md sm:left-7 sm:top-7 dark:bg-white/[.055] dark:text-[#bdbdbd]">
        <span className="h-2 w-2 rounded-full bg-[#525252] dark:bg-[#d4d4d4]" />
        多源信号汇聚
      </div>

      <div className="pointer-events-none absolute bottom-6 left-5 space-y-2 sm:bottom-8 sm:left-7">
        {["X / Twitter", "RSS / Atom", "Hacker News"].map((source, index) => (
          <div
            key={source}
            className="signal-orbit-label signal-orbit-source flex items-center gap-2 text-[11px] font-medium text-[#666666] dark:text-[#a3a3a3]"
          >
            <span
              className="h-1.5 w-1.5 rounded-full"
              style={{
                backgroundColor: ["#525252", "#858585", "#b8b8b8"][index],
              }}
            />
            {source}
          </div>
        ))}
      </div>

      <div className="signal-orbit-label signal-orbit-heat pointer-events-none absolute bottom-6 right-5 rounded-xl bg-white/75 p-3.5 text-right backdrop-blur-md sm:bottom-8 sm:right-7 sm:p-4 dark:bg-white/[.055]">
        <p className="text-[10px] font-semibold uppercase tracking-[0.13em] text-[#666666] dark:text-[#a3a3a3]">
          事件热度上升
        </p>
        <p className="mt-1 font-mono text-2xl font-semibold text-[#262626] dark:text-[#d4d4d4]">
          92
          <span className="ml-1 text-[10px] font-medium text-[#666666]">
            /100
          </span>
        </p>
      </div>
    </figure>
  );
}
