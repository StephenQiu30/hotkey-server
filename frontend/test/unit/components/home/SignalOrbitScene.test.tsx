import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SignalOrbitScene } from "@/components/home/SignalOrbitScene";

const threeMock = vi.hoisted(() => {
  class VectorLike {
    x = 0;
    y = 0;
    z = 0;
    set = vi.fn((x = 0, y = 0, z = 0) => {
      this.x = x;
      this.y = y;
      this.z = z;
    });
  }

  class ObjectLike {
    position = new VectorLike();
    rotation = new VectorLike();
    add = vi.fn();
  }

  class Disposable {
    dispose = vi.fn();
  }

  const runtime = {
    moduleLoaded: vi.fn(),
    renderers: [] as WebGLRenderer[],
    throwOnRender: false,
  };

  class WebGLRenderer {
    dispose = vi.fn();
    forceContextLoss = vi.fn();
    render = vi.fn(() => {
      if (runtime.throwOnRender) throw new Error("render failed");
    });
    setAnimationLoop = vi.fn();
    setClearColor = vi.fn();
    setPixelRatio = vi.fn();
    setSize = vi.fn();
    outputColorSpace = "";

    constructor() {
      runtime.renderers.push(this);
    }
  }

  class Scene extends ObjectLike {
    clear = vi.fn();
  }

  class PerspectiveCamera extends ObjectLike {
    aspect = 1;
    updateProjectionMatrix = vi.fn();
  }

  class BufferGeometry extends Disposable {
    setAttribute = vi.fn();
  }

  class Material extends Disposable {}
  class Mesh extends ObjectLike {}
  class Group extends ObjectLike {}
  class Points extends ObjectLike {}
  class LineLoop extends ObjectLike {}
  class PointLight extends ObjectLike {}
  class AmbientLight extends ObjectLike {}

  const module = {
    AdditiveBlending: 1,
    DoubleSide: 2,
    SRGBColorSpace: "srgb",
    AmbientLight,
    BufferAttribute: class {},
    BufferGeometry,
    Color: class {
      r = 0.3;
      g = 0.6;
      b = 0.9;
    },
    Float32BufferAttribute: class {},
    Group,
    IcosahedronGeometry: class extends Disposable {},
    LineBasicMaterial: Material,
    LineLoop,
    Mesh,
    MeshBasicMaterial: Material,
    MeshStandardMaterial: Material,
    PerspectiveCamera,
    PointLight,
    Points,
    PointsMaterial: Material,
    RingGeometry: class extends Disposable {},
    Scene,
    SphereGeometry: class extends Disposable {},
    TorusGeometry: class extends Disposable {},
    WebGLRenderer,
  };

  return { module, runtime };
});

vi.mock("three", () => {
  threeMock.runtime.moduleLoaded();
  return threeMock.module;
});

type MediaState = {
  compact?: boolean;
  reduceMotion?: boolean;
};

function installMatchMedia({
  compact = false,
  reduceMotion = false,
}: MediaState = {}) {
  const queries = new Map<string, MediaQueryList>();

  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn((query: string) => {
      const existing = queries.get(query);
      if (existing) return existing;

      const matches = query.includes("prefers-reduced-motion: reduce")
        ? reduceMotion
        : query.includes("prefers-reduced-motion: no-preference")
        ? !reduceMotion
        : query.includes("max-width: 767px")
        ? compact
        : false;
      const mediaQuery = {
        matches,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(() => false),
      } as unknown as MediaQueryList;
      queries.set(query, mediaQuery);
      return mediaQuery;
    }),
  });

  return queries;
}

function setPerformanceHints({
  cores = 8,
  memory = 8,
  saveData = false,
}: {
  cores?: number;
  memory?: number;
  saveData?: boolean;
} = {}) {
  Object.defineProperties(navigator, {
    connection: {
      configurable: true,
      value: { effectiveType: "4g", saveData },
    },
    deviceMemory: { configurable: true, value: memory },
    hardwareConcurrency: { configurable: true, value: cores },
  });
}

function enableWebGL() {
  Object.defineProperty(window, "WebGL2RenderingContext", {
    configurable: true,
    value: class {},
  });
  const loseContext = vi.fn();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(
    (() => ({
      getExtension: () => ({ loseContext }),
    })) as never
  );
  return loseContext;
}

function installAnimationObservers() {
  let intersectionCallback: IntersectionObserverCallback | undefined;
  const intersectionDisconnect = vi.fn();
  const resizeDisconnect = vi.fn();
  const requestAnimationFrame = vi.fn(() => 17);
  const cancelAnimationFrame = vi.fn();

  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback;
      }
      disconnect = intersectionDisconnect;
      observe = vi.fn();
    }
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      disconnect = resizeDisconnect;
      observe = vi.fn();
    }
  );
  vi.stubGlobal("requestAnimationFrame", requestAnimationFrame);
  vi.stubGlobal("cancelAnimationFrame", cancelAnimationFrame);
  vi.spyOn(document, "hidden", "get").mockReturnValue(false);

  return {
    cancelAnimationFrame,
    getIntersectionCallback: () => intersectionCallback,
    intersectionDisconnect,
    requestAnimationFrame,
    resizeDisconnect,
  };
}

describe("SignalOrbitScene", () => {
  beforeEach(() => {
    delete (window as { WebGL2RenderingContext?: unknown })
      .WebGL2RenderingContext;
    installMatchMedia();
    setPerformanceHints();
    threeMock.runtime.renderers.length = 0;
    threeMock.runtime.throwOnRender = false;
  });

  afterEach(() => {
    delete (window as { WebGL2RenderingContext?: unknown })
      .WebGL2RenderingContext;
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("keeps an accessible optimized fallback around the decorative canvas", () => {
    render(<SignalOrbitScene />);

    const scene = screen.getByRole("img", {
      name: "多来源信号汇聚成热点事件的动态轨迹",
    });
    expect(scene).toHaveAttribute("data-animation", "gsap-three");
    expect(scene).toHaveAttribute("data-variant", "panel");
    expect(scene).toHaveAttribute("data-webgl", "fallback");
    expect(scene.querySelector("canvas")).toHaveAttribute(
      "aria-hidden",
      "true"
    );
    expect(scene.querySelector("canvas")).toHaveStyle({
      opacity: "0",
      visibility: "hidden",
    });
    expect(
      scene.querySelector('img[src*="hotkey-signal-radar"]')
    ).toHaveAttribute(
      "sizes",
      "(max-width: 640px) 92vw, (max-width: 1024px) 72vw, 46vw"
    );
    expect(screen.getByText("多源信号汇聚")).toBeInTheDocument();
    expect(screen.getByText("事件热度上升")).toBeInTheDocument();
  });

  it("supports an ambient borderless treatment for low-density pages", () => {
    render(<SignalOrbitScene variant="ambient" />);

    const scene = screen.getByRole("img", {
      name: "多来源信号汇聚成热点事件的动态轨迹",
    });
    expect(scene).toHaveAttribute("data-variant", "ambient");
    expect(scene).toHaveClass("bg-transparent", "shadow-none");
    expect(scene).not.toHaveClass("rounded-2xl", "bg-[#f2f2f2]");
    expect(scene.querySelector("canvas")).not.toHaveClass(
      "transition-opacity",
      "duration-500"
    );
  });

  it.each([
    ["reduced motion", { reduceMotion: true }, {}],
    ["save data", {}, { saveData: true }],
    ["low-end compact device", { compact: true }, { cores: 4 }],
  ])("does not import Three.js for %s", async (_label, media, hints) => {
    installMatchMedia(media);
    setPerformanceHints(hints);
    enableWebGL();

    render(<SignalOrbitScene />);
    await act(async () => undefined);

    expect(threeMock.runtime.moduleLoaded).not.toHaveBeenCalled();
    expect(
      screen.getByRole("img", {
        name: "多来源信号汇聚成热点事件的动态轨迹",
      })
    ).toHaveAttribute("data-webgl", "fallback");
  });

  it("pauses offscreen, falls back on context loss, restores, and disposes", async () => {
    enableWebGL();
    const observers = installAnimationObservers();
    const { unmount } = render(<SignalOrbitScene />);

    await waitFor(() => expect(threeMock.runtime.renderers).toHaveLength(1));
    const renderer = threeMock.runtime.renderers[0];
    const scene = screen.getByRole("img", {
      name: "多来源信号汇聚成热点事件的动态轨迹",
    });
    const canvas = scene.querySelector("canvas");
    expect(canvas).not.toBeNull();
    expect(scene).toHaveAttribute("data-webgl", "ready");
    expect(observers.requestAnimationFrame).toHaveBeenCalled();

    act(() => {
      observers.getIntersectionCallback()?.(
        [
          { isIntersecting: false, intersectionRatio: 0 },
        ] as IntersectionObserverEntry[],
        {} as IntersectionObserver
      );
    });
    expect(observers.cancelAnimationFrame).toHaveBeenCalledWith(17);

    act(() => {
      observers.getIntersectionCallback()?.(
        [
          { isIntersecting: true, intersectionRatio: 1 },
        ] as IntersectionObserverEntry[],
        {} as IntersectionObserver
      );
    });
    expect(observers.requestAnimationFrame).toHaveBeenCalledTimes(2);

    const lostEvent = new Event("webglcontextlost", { cancelable: true });
    act(() => canvas?.dispatchEvent(lostEvent));
    expect(lostEvent.defaultPrevented).toBe(true);
    expect(scene).toHaveAttribute("data-webgl", "fallback");

    act(() => canvas?.dispatchEvent(new Event("webglcontextrestored")));
    expect(scene).toHaveAttribute("data-webgl", "ready");

    unmount();
    expect(renderer.dispose).toHaveBeenCalledOnce();
    expect(renderer.forceContextLoss).toHaveBeenCalledOnce();
    expect(observers.intersectionDisconnect).toHaveBeenCalledOnce();
    expect(observers.resizeDisconnect).toHaveBeenCalledOnce();
  });

  it("tears down partial initialization when the first render fails", async () => {
    enableWebGL();
    installAnimationObservers();
    threeMock.runtime.throwOnRender = true;
    render(<SignalOrbitScene />);

    await waitFor(() => expect(threeMock.runtime.renderers).toHaveLength(1));
    const renderer = threeMock.runtime.renderers[0];
    await waitFor(() => expect(renderer.dispose).toHaveBeenCalledOnce());
    expect(renderer.forceContextLoss).toHaveBeenCalledOnce();
    expect(
      screen.getByRole("img", {
        name: "多来源信号汇聚成热点事件的动态轨迹",
      })
    ).toHaveAttribute("data-webgl", "fallback");
  });
});
