(function() {
  "use strict";
  /**
  * @vue/shared v3.5.42
  * (c) 2018-present Yuxi (Evan) You and Vue contributors
  * @license MIT
  **/
  // @__NO_SIDE_EFFECTS__
  function makeMap(str) {
    const map = /* @__PURE__ */ Object.create(null);
    for (const key of str.split(",")) map[key] = 1;
    return (val) => val in map;
  }
  const EMPTY_OBJ = {};
  const EMPTY_ARR = [];
  const NOOP = () => {
  };
  const NO = () => false;
  const isOn = (key) => key.charCodeAt(0) === 111 && key.charCodeAt(1) === 110 && // uppercase letter
  (key.charCodeAt(2) > 122 || key.charCodeAt(2) < 97);
  const isModelListener = (key) => key.startsWith("onUpdate:");
  const extend = Object.assign;
  const remove = (arr, el2) => {
    const i = arr.indexOf(el2);
    if (i > -1) {
      arr.splice(i, 1);
    }
  };
  const hasOwnProperty$1 = Object.prototype.hasOwnProperty;
  const hasOwn = (val, key) => hasOwnProperty$1.call(val, key);
  const isArray = Array.isArray;
  const isMap = (val) => toTypeString(val) === "[object Map]";
  const isSet = (val) => toTypeString(val) === "[object Set]";
  const isDate = (val) => toTypeString(val) === "[object Date]";
  const isFunction = (val) => typeof val === "function";
  const isString = (val) => typeof val === "string";
  const isSymbol = (val) => typeof val === "symbol";
  const isObject = (val) => val !== null && typeof val === "object";
  const isPromise = (val) => {
    return (isObject(val) || isFunction(val)) && isFunction(val.then) && isFunction(val.catch);
  };
  const objectToString = Object.prototype.toString;
  const toTypeString = (value) => objectToString.call(value);
  const toRawType = (value) => {
    return toTypeString(value).slice(8, -1);
  };
  const isPlainObject = (val) => toTypeString(val) === "[object Object]";
  const isIntegerKey = (key) => isString(key) && key !== "NaN" && key[0] !== "-" && "" + parseInt(key, 10) === key;
  const isReservedProp = /* @__PURE__ */ makeMap(
    // the leading comma is intentional so empty string "" is also included
    ",key,ref,ref_for,ref_key,onVnodeBeforeMount,onVnodeMounted,onVnodeBeforeUpdate,onVnodeUpdated,onVnodeBeforeUnmount,onVnodeUnmounted"
  );
  const cacheStringFunction = (fn) => {
    const cache = /* @__PURE__ */ Object.create(null);
    return ((str) => {
      const hit = cache[str];
      return hit || (cache[str] = fn(str));
    });
  };
  const camelizeRE = /-\w/g;
  const camelize = cacheStringFunction(
    (str) => {
      return str.replace(camelizeRE, (c) => c.slice(1).toUpperCase());
    }
  );
  const hyphenateRE = /\B([A-Z])/g;
  const hyphenate = cacheStringFunction(
    (str) => str.replace(hyphenateRE, "-$1").toLowerCase()
  );
  const capitalize = cacheStringFunction((str) => {
    return str.charAt(0).toUpperCase() + str.slice(1);
  });
  const toHandlerKey = cacheStringFunction(
    (str) => {
      const s = str ? `on${capitalize(str)}` : ``;
      return s;
    }
  );
  const hasChanged = (value, oldValue) => !Object.is(value, oldValue);
  const invokeArrayFns = (fns, ...arg) => {
    for (let i = 0; i < fns.length; i++) {
      fns[i](...arg);
    }
  };
  const def = (obj, key, value, writable = false) => {
    Object.defineProperty(obj, key, {
      configurable: true,
      enumerable: false,
      writable,
      value
    });
  };
  const looseToNumber = (val) => {
    const n = parseFloat(val);
    return isNaN(n) ? val : n;
  };
  let _globalThis;
  const getGlobalThis = () => {
    return _globalThis || (_globalThis = typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : typeof global !== "undefined" ? global : {});
  };
  function normalizeStyle(value) {
    if (isArray(value)) {
      const res = {};
      for (let i = 0; i < value.length; i++) {
        const item = value[i];
        const normalized = isString(item) ? parseStringStyle(item) : normalizeStyle(item);
        if (normalized) {
          for (const key in normalized) {
            res[key] = normalized[key];
          }
        }
      }
      return res;
    } else if (isString(value) || isObject(value)) {
      return value;
    }
  }
  const listDelimiterRE = /;(?![^(]*\))/g;
  const propertyDelimiterRE = /:([^]+)/;
  const styleCommentRE = /\/\*[^]*?\*\//g;
  function parseStringStyle(cssText) {
    const ret = {};
    cssText.replace(styleCommentRE, "").split(listDelimiterRE).forEach((item) => {
      if (item) {
        const tmp = item.split(propertyDelimiterRE);
        tmp.length > 1 && (ret[tmp[0].trim()] = tmp[1].trim());
      }
    });
    return ret;
  }
  function normalizeClass(value) {
    let res = "";
    if (isString(value)) {
      res = value;
    } else if (isArray(value)) {
      for (let i = 0; i < value.length; i++) {
        const normalized = normalizeClass(value[i]);
        if (normalized) {
          res += normalized + " ";
        }
      }
    } else if (isObject(value)) {
      for (const name in value) {
        if (value[name]) {
          res += name + " ";
        }
      }
    }
    return res.trim();
  }
  const specialBooleanAttrs = `itemscope,allowfullscreen,formnovalidate,ismap,nomodule,novalidate,readonly`;
  const isSpecialBooleanAttr = /* @__PURE__ */ makeMap(specialBooleanAttrs);
  function includeBooleanAttr(value) {
    return !!value || value === "";
  }
  function looseCompareArrays(a, b) {
    if (a.length !== b.length) return false;
    let equal = true;
    for (let i = 0; equal && i < a.length; i++) {
      equal = looseEqual(a[i], b[i]);
    }
    return equal;
  }
  function looseCompareCollections(a, b) {
    if (a.size !== b.size) return false;
    const candidates = Array.from(b);
    const matched = new Uint8Array(candidates.length);
    for (const item of a) {
      let index = -1;
      for (let i = 0; i < candidates.length; i++) {
        if (!matched[i] && looseEqual(item, candidates[i])) {
          index = i;
          break;
        }
      }
      if (index < 0) return false;
      matched[index] = 1;
    }
    return true;
  }
  function looseEqual(a, b) {
    if (a === b) return true;
    let aValidType = isDate(a);
    let bValidType = isDate(b);
    if (aValidType || bValidType) {
      return aValidType && bValidType ? a.getTime() === b.getTime() : false;
    }
    aValidType = isSymbol(a);
    bValidType = isSymbol(b);
    if (aValidType || bValidType) {
      return a === b;
    }
    aValidType = isArray(a);
    bValidType = isArray(b);
    if (aValidType || bValidType) {
      return aValidType && bValidType ? looseCompareArrays(a, b) : false;
    }
    aValidType = isObject(a);
    bValidType = isObject(b);
    if (aValidType || bValidType) {
      if (!aValidType || !bValidType) {
        return false;
      }
      aValidType = isMap(a);
      bValidType = isMap(b);
      if (aValidType || bValidType) {
        return aValidType && bValidType ? looseCompareCollections(a, b) : false;
      }
      aValidType = isSet(a);
      bValidType = isSet(b);
      if (aValidType || bValidType) {
        return aValidType && bValidType ? looseCompareCollections(a, b) : false;
      }
      const aKeysCount = Object.keys(a).length;
      const bKeysCount = Object.keys(b).length;
      if (aKeysCount !== bKeysCount) {
        return false;
      }
      for (const key in a) {
        const aHasKey = a.hasOwnProperty(key);
        const bHasKey = b.hasOwnProperty(key);
        if (aHasKey && !bHasKey || !aHasKey && bHasKey || !looseEqual(a[key], b[key])) {
          return false;
        }
      }
    }
    return String(a) === String(b);
  }
  const isRef$1 = (val) => {
    return !!(val && val["__v_isRef"] === true);
  };
  const toDisplayString = (val) => {
    return isString(val) ? val : val == null ? "" : isArray(val) || isObject(val) && (val.toString === objectToString || !isFunction(val.toString)) ? isRef$1(val) ? toDisplayString(val.value) : JSON.stringify(val, replacer, 2) : String(val);
  };
  const replacer = (_key, val) => {
    if (isRef$1(val)) {
      return replacer(_key, val.value);
    } else if (isMap(val)) {
      return {
        [`Map(${val.size})`]: [...val.entries()].reduce(
          (entries, [key, val2], i) => {
            entries[stringifySymbol(key, i) + " =>"] = val2;
            return entries;
          },
          {}
        )
      };
    } else if (isSet(val)) {
      return {
        [`Set(${val.size})`]: [...val.values()].map((v) => stringifySymbol(v))
      };
    } else if (isSymbol(val)) {
      return stringifySymbol(val);
    } else if (isObject(val) && !isArray(val) && !isPlainObject(val)) {
      return String(val);
    }
    return val;
  };
  const stringifySymbol = (v, i = "") => {
    var _a;
    return (
      // Symbol.description in es2019+ so we need to cast here to pass
      // the lib: es2016 check
      isSymbol(v) ? `Symbol(${(_a = v.description) != null ? _a : i})` : v
    );
  };
  /**
  * @vue/reactivity v3.5.42
  * (c) 2018-present Yuxi (Evan) You and Vue contributors
  * @license MIT
  **/
  let activeEffectScope;
  class EffectScope {
    // TODO isolatedDeclarations "__v_skip"
    constructor(detached = false) {
      this.detached = detached;
      this._active = true;
      this._on = 0;
      this.effects = [];
      this.cleanups = [];
      this._isPaused = false;
      this._warnOnRun = true;
      this.__v_skip = true;
      if (!detached && activeEffectScope) {
        if (activeEffectScope.active) {
          this.parent = activeEffectScope;
          this.index = (activeEffectScope.scopes || (activeEffectScope.scopes = [])).push(
            this
          ) - 1;
        } else {
          this._active = false;
          this._warnOnRun = false;
        }
      }
    }
    get active() {
      return this._active;
    }
    pause() {
      if (this._active) {
        this._isPaused = true;
        let i, l;
        if (this.scopes) {
          const scopes = this.scopes.slice();
          for (i = 0, l = scopes.length; i < l; i++) {
            scopes[i].pause();
          }
        }
        for (i = 0, l = this.effects.length; i < l; i++) {
          this.effects[i].pause();
        }
      }
    }
    /**
     * Resumes the effect scope, including all child scopes and effects.
     */
    resume() {
      if (this._active) {
        if (this._isPaused) {
          this._isPaused = false;
          let i, l;
          if (this.scopes) {
            const scopes = this.scopes.slice();
            for (i = 0, l = scopes.length; i < l; i++) {
              scopes[i].resume();
            }
          }
          const effects = this.effects.slice();
          for (i = 0, l = effects.length; i < l; i++) {
            effects[i].resume();
          }
        }
      }
    }
    run(fn) {
      if (this._active) {
        const currentEffectScope = activeEffectScope;
        try {
          activeEffectScope = this;
          return fn();
        } finally {
          activeEffectScope = currentEffectScope;
        }
      }
    }
    /**
     * This should only be called on non-detached scopes
     * @internal
     */
    on() {
      if (++this._on === 1) {
        this.prevScope = activeEffectScope;
        activeEffectScope = this;
      }
    }
    /**
     * This should only be called on non-detached scopes
     * @internal
     */
    off() {
      if (this._on > 0 && --this._on === 0) {
        if (activeEffectScope === this) {
          activeEffectScope = this.prevScope;
        } else {
          let current = activeEffectScope;
          while (current) {
            if (current.prevScope === this) {
              current.prevScope = this.prevScope;
              break;
            }
            current = current.prevScope;
          }
        }
        this.prevScope = void 0;
      }
    }
    stop(fromParent) {
      if (this._active) {
        this._active = false;
        let i, l;
        for (i = 0, l = this.effects.length; i < l; i++) {
          this.effects[i].stop();
        }
        this.effects.length = 0;
        for (i = 0, l = this.cleanups.length; i < l; i++) {
          this.cleanups[i]();
        }
        this.cleanups.length = 0;
        if (this.scopes) {
          const scopes = this.scopes.slice();
          for (i = 0, l = scopes.length; i < l; i++) {
            scopes[i].stop(true);
          }
          this.scopes.length = 0;
        }
        if (!this.detached && this.parent && !fromParent) {
          const last = this.parent.scopes.pop();
          if (last && last !== this) {
            this.parent.scopes[this.index] = last;
            last.index = this.index;
          }
        }
        this.parent = void 0;
      }
    }
  }
  function getCurrentScope() {
    return activeEffectScope;
  }
  let activeSub;
  const pausedQueueEffects = /* @__PURE__ */ new WeakSet();
  class ReactiveEffect {
    constructor(fn) {
      this.fn = fn;
      this.deps = void 0;
      this.depsTail = void 0;
      this.flags = 1 | 4;
      this.next = void 0;
      this.cleanup = void 0;
      this.scheduler = void 0;
      if (activeEffectScope) {
        if (activeEffectScope.active) {
          activeEffectScope.effects.push(this);
        } else {
          this.flags &= -2;
        }
      }
    }
    pause() {
      this.flags |= 64;
    }
    resume() {
      if (this.flags & 64) {
        this.flags &= -65;
        if (pausedQueueEffects.has(this)) {
          pausedQueueEffects.delete(this);
          this.trigger();
        }
      }
    }
    /**
     * @internal
     */
    notify() {
      if (this.flags & 2 && !(this.flags & 32)) {
        return;
      }
      if (!(this.flags & 8)) {
        batch(this);
      }
    }
    run() {
      if (!(this.flags & 1)) {
        return this.fn();
      }
      this.flags |= 2;
      cleanupEffect(this);
      prepareDeps(this);
      const prevEffect = activeSub;
      const prevShouldTrack = shouldTrack;
      activeSub = this;
      shouldTrack = true;
      try {
        return this.fn();
      } finally {
        cleanupDeps(this);
        activeSub = prevEffect;
        shouldTrack = prevShouldTrack;
        this.flags &= -3;
      }
    }
    stop() {
      if (this.flags & 1) {
        for (let link = this.deps; link; link = link.nextDep) {
          removeSub(link);
        }
        this.deps = this.depsTail = void 0;
        cleanupEffect(this);
        this.onStop && this.onStop();
        this.flags &= -2;
      }
    }
    trigger() {
      if (this.flags & 64) {
        pausedQueueEffects.add(this);
      } else if (this.scheduler) {
        this.scheduler();
      } else {
        this.runIfDirty();
      }
    }
    /**
     * @internal
     */
    runIfDirty() {
      if (isDirty(this)) {
        this.run();
      }
    }
    get dirty() {
      return isDirty(this);
    }
  }
  let batchDepth = 0;
  let batchedSub;
  let batchedComputed;
  function batch(sub, isComputed = false) {
    sub.flags |= 8;
    if (isComputed) {
      sub.next = batchedComputed;
      batchedComputed = sub;
      return;
    }
    sub.next = batchedSub;
    batchedSub = sub;
  }
  function startBatch() {
    batchDepth++;
  }
  function endBatch() {
    if (--batchDepth > 0) {
      return;
    }
    if (batchedComputed) {
      let e = batchedComputed;
      batchedComputed = void 0;
      while (e) {
        const next = e.next;
        e.next = void 0;
        e.flags &= -9;
        e = next;
      }
    }
    let error;
    while (batchedSub) {
      let e = batchedSub;
      batchedSub = void 0;
      while (e) {
        const next = e.next;
        e.next = void 0;
        e.flags &= -9;
        if (e.flags & 1) {
          try {
            ;
            e.trigger();
          } catch (err) {
            if (!error) error = err;
          }
        }
        e = next;
      }
    }
    if (error) throw error;
  }
  function prepareDeps(sub) {
    for (let link = sub.deps; link; link = link.nextDep) {
      link.version = -1;
      link.prevActiveLink = link.dep.activeLink;
      link.dep.activeLink = link;
    }
  }
  function cleanupDeps(sub) {
    let head;
    let tail = sub.depsTail;
    let link = tail;
    while (link) {
      const prev = link.prevDep;
      if (link.version === -1) {
        if (link === tail) tail = prev;
        removeSub(link);
        removeDep(link);
      } else {
        head = link;
      }
      link.dep.activeLink = link.prevActiveLink;
      link.prevActiveLink = void 0;
      link = prev;
    }
    sub.deps = head;
    sub.depsTail = tail;
  }
  function isDirty(sub) {
    for (let link = sub.deps; link; link = link.nextDep) {
      if (link.dep.version !== link.version || link.dep.computed && (refreshComputed(link.dep.computed) || link.dep.version !== link.version)) {
        return true;
      }
    }
    if (sub._dirty) {
      return true;
    }
    return false;
  }
  function refreshComputed(computed2) {
    if (computed2.flags & 4 && !(computed2.flags & 16)) {
      return;
    }
    computed2.flags &= -17;
    if (computed2.globalVersion === globalVersion) {
      return;
    }
    computed2.globalVersion = globalVersion;
    if (!computed2.isSSR && computed2.flags & 128 && (!computed2.deps && !computed2._dirty || !isDirty(computed2))) {
      return;
    }
    computed2.flags |= 2;
    const dep = computed2.dep;
    const prevSub = activeSub;
    const prevShouldTrack = shouldTrack;
    activeSub = computed2;
    shouldTrack = true;
    try {
      prepareDeps(computed2);
      const value = computed2.fn(computed2._value);
      if (dep.version === 0 || hasChanged(value, computed2._value)) {
        computed2.flags |= 128;
        computed2._value = value;
        dep.version++;
      }
    } catch (err) {
      dep.version++;
      throw err;
    } finally {
      activeSub = prevSub;
      shouldTrack = prevShouldTrack;
      cleanupDeps(computed2);
      computed2.flags &= -3;
    }
  }
  function removeSub(link, soft = false) {
    const { dep, prevSub, nextSub } = link;
    if (prevSub) {
      prevSub.nextSub = nextSub;
      link.prevSub = void 0;
    }
    if (nextSub) {
      nextSub.prevSub = prevSub;
      link.nextSub = void 0;
    }
    if (dep.subs === link) {
      dep.subs = prevSub;
      if (!prevSub && dep.computed) {
        dep.computed.flags &= -5;
        for (let l = dep.computed.deps; l; l = l.nextDep) {
          removeSub(l, true);
        }
      }
    }
    if (!soft && !--dep.sc && dep.map) {
      dep.map.delete(dep.key);
    }
  }
  function removeDep(link) {
    const { prevDep, nextDep } = link;
    if (prevDep) {
      prevDep.nextDep = nextDep;
      link.prevDep = void 0;
    }
    if (nextDep) {
      nextDep.prevDep = prevDep;
      link.nextDep = void 0;
    }
  }
  let shouldTrack = true;
  const trackStack = [];
  function pauseTracking() {
    trackStack.push(shouldTrack);
    shouldTrack = false;
  }
  function resetTracking() {
    const last = trackStack.pop();
    shouldTrack = last === void 0 ? true : last;
  }
  function cleanupEffect(e) {
    const { cleanup } = e;
    e.cleanup = void 0;
    if (cleanup) {
      const prevSub = activeSub;
      activeSub = void 0;
      try {
        cleanup();
      } finally {
        activeSub = prevSub;
      }
    }
  }
  let globalVersion = 0;
  class Link {
    constructor(sub, dep) {
      this.sub = sub;
      this.dep = dep;
      this.version = dep.version;
      this.nextDep = this.prevDep = this.nextSub = this.prevSub = this.prevActiveLink = void 0;
    }
  }
  class Dep {
    // TODO isolatedDeclarations "__v_skip"
    constructor(computed2) {
      this.computed = computed2;
      this.version = 0;
      this.activeLink = void 0;
      this.subs = void 0;
      this.map = void 0;
      this.key = void 0;
      this.sc = 0;
      this.__v_skip = true;
    }
    track(debugInfo) {
      if (!activeSub || !shouldTrack || activeSub === this.computed) {
        return;
      }
      let link = this.activeLink;
      if (link === void 0 || link.sub !== activeSub) {
        link = this.activeLink = new Link(activeSub, this);
        if (!activeSub.deps) {
          activeSub.deps = activeSub.depsTail = link;
        } else {
          link.prevDep = activeSub.depsTail;
          activeSub.depsTail.nextDep = link;
          activeSub.depsTail = link;
        }
        addSub(link);
      } else if (link.version === -1) {
        link.version = this.version;
        if (link.nextDep) {
          const next = link.nextDep;
          next.prevDep = link.prevDep;
          if (link.prevDep) {
            link.prevDep.nextDep = next;
          }
          link.prevDep = activeSub.depsTail;
          link.nextDep = void 0;
          activeSub.depsTail.nextDep = link;
          activeSub.depsTail = link;
          if (activeSub.deps === link) {
            activeSub.deps = next;
          }
        }
      }
      return link;
    }
    trigger(debugInfo) {
      this.version++;
      globalVersion++;
      this.notify(debugInfo);
    }
    notify(debugInfo) {
      startBatch();
      try {
        if (false) ;
        for (let link = this.subs; link; link = link.prevSub) {
          if (link.sub.notify()) {
            ;
            link.sub.dep.notify();
          }
        }
      } finally {
        endBatch();
      }
    }
  }
  function addSub(link) {
    link.dep.sc++;
    if (link.sub.flags & 4) {
      const computed2 = link.dep.computed;
      if (computed2 && !link.dep.subs) {
        computed2.flags |= 4 | 16;
        for (let l = computed2.deps; l; l = l.nextDep) {
          addSub(l);
        }
      }
      const currentTail = link.dep.subs;
      if (currentTail !== link) {
        link.prevSub = currentTail;
        if (currentTail) currentTail.nextSub = link;
      }
      link.dep.subs = link;
    }
  }
  const targetMap = /* @__PURE__ */ new WeakMap();
  const ITERATE_KEY = /* @__PURE__ */ Symbol(
    ""
  );
  const MAP_KEY_ITERATE_KEY = /* @__PURE__ */ Symbol(
    ""
  );
  const ARRAY_ITERATE_KEY = /* @__PURE__ */ Symbol(
    ""
  );
  function track(target, type, key) {
    if (shouldTrack && activeSub) {
      let depsMap = targetMap.get(target);
      if (!depsMap) {
        targetMap.set(target, depsMap = /* @__PURE__ */ new Map());
      }
      let dep = depsMap.get(key);
      if (!dep) {
        depsMap.set(key, dep = new Dep());
        dep.map = depsMap;
        dep.key = key;
      }
      {
        dep.track();
      }
    }
  }
  function trigger(target, type, key, newValue, oldValue, oldTarget) {
    const depsMap = targetMap.get(target);
    if (!depsMap) {
      globalVersion++;
      return;
    }
    const run = (dep) => {
      if (dep) {
        {
          dep.trigger();
        }
      }
    };
    startBatch();
    if (type === "clear") {
      depsMap.forEach(run);
    } else {
      const targetIsArray = isArray(target);
      const isArrayIndex = targetIsArray && isIntegerKey(key);
      if (targetIsArray && key === "length") {
        const newLength = Number(newValue);
        depsMap.forEach((dep, key2) => {
          if (key2 === "length" || key2 === ARRAY_ITERATE_KEY || !isSymbol(key2) && key2 >= newLength) {
            run(dep);
          }
        });
      } else {
        if (key !== void 0 || depsMap.has(void 0)) {
          run(depsMap.get(key));
        }
        if (isArrayIndex) {
          run(depsMap.get(ARRAY_ITERATE_KEY));
        }
        switch (type) {
          case "add":
            if (!targetIsArray) {
              run(depsMap.get(ITERATE_KEY));
              if (isMap(target)) {
                run(depsMap.get(MAP_KEY_ITERATE_KEY));
              }
            } else if (isArrayIndex) {
              run(depsMap.get("length"));
            }
            break;
          case "delete":
            if (!targetIsArray) {
              run(depsMap.get(ITERATE_KEY));
              if (isMap(target)) {
                run(depsMap.get(MAP_KEY_ITERATE_KEY));
              }
            }
            break;
          case "set":
            if (isMap(target)) {
              run(depsMap.get(ITERATE_KEY));
            }
            break;
        }
      }
    }
    endBatch();
  }
  function reactiveReadArray(array) {
    const raw = /* @__PURE__ */ toRaw(array);
    if (raw === array) return raw;
    track(raw, "iterate", ARRAY_ITERATE_KEY);
    return /* @__PURE__ */ isShallow(array) ? raw : raw.map(toReactive);
  }
  function shallowReadArray(arr) {
    track(arr = /* @__PURE__ */ toRaw(arr), "iterate", ARRAY_ITERATE_KEY);
    return arr;
  }
  function toWrapped(target, item) {
    if (/* @__PURE__ */ isReadonly(target)) {
      return /* @__PURE__ */ isReactive(target) ? toReadonly(toReactive(item)) : toReadonly(item);
    }
    return toReactive(item);
  }
  const arrayInstrumentations = {
    __proto__: null,
    [Symbol.iterator]() {
      return iterator(this, Symbol.iterator, (item) => toWrapped(this, item));
    },
    concat(...args) {
      return reactiveReadArray(this).concat(
        ...args.map((x) => isArray(x) ? reactiveReadArray(x) : x)
      );
    },
    entries() {
      return iterator(this, "entries", (value) => {
        value[1] = toWrapped(this, value[1]);
        return value;
      });
    },
    every(fn, thisArg) {
      return apply(this, "every", fn, thisArg, void 0, arguments);
    },
    filter(fn, thisArg) {
      return apply(
        this,
        "filter",
        fn,
        thisArg,
        (v) => v.map((item) => toWrapped(this, item)),
        arguments
      );
    },
    find(fn, thisArg) {
      return apply(
        this,
        "find",
        fn,
        thisArg,
        (item) => toWrapped(this, item),
        arguments
      );
    },
    findIndex(fn, thisArg) {
      return apply(this, "findIndex", fn, thisArg, void 0, arguments);
    },
    findLast(fn, thisArg) {
      return apply(
        this,
        "findLast",
        fn,
        thisArg,
        (item) => toWrapped(this, item),
        arguments
      );
    },
    findLastIndex(fn, thisArg) {
      return apply(this, "findLastIndex", fn, thisArg, void 0, arguments);
    },
    // flat, flatMap could benefit from ARRAY_ITERATE but are not straight-forward to implement
    forEach(fn, thisArg) {
      return apply(this, "forEach", fn, thisArg, void 0, arguments);
    },
    includes(...args) {
      return searchProxy(this, "includes", args);
    },
    indexOf(...args) {
      return searchProxy(this, "indexOf", args);
    },
    join(separator) {
      return reactiveReadArray(this).join(separator);
    },
    // keys() iterator only reads `length`, no optimization required
    lastIndexOf(...args) {
      return searchProxy(this, "lastIndexOf", args);
    },
    map(fn, thisArg) {
      return apply(this, "map", fn, thisArg, void 0, arguments);
    },
    pop() {
      return noTracking(this, "pop");
    },
    push(...args) {
      return noTracking(this, "push", args);
    },
    reduce(fn, ...args) {
      return reduce(this, "reduce", fn, args);
    },
    reduceRight(fn, ...args) {
      return reduce(this, "reduceRight", fn, args);
    },
    shift() {
      return noTracking(this, "shift");
    },
    // slice could use ARRAY_ITERATE but also seems to beg for range tracking
    some(fn, thisArg) {
      return apply(this, "some", fn, thisArg, void 0, arguments);
    },
    splice(...args) {
      return noTracking(this, "splice", args);
    },
    toReversed() {
      return reactiveReadArray(this).toReversed();
    },
    toSorted(comparer) {
      return reactiveReadArray(this).toSorted(comparer);
    },
    toSpliced(...args) {
      return reactiveReadArray(this).toSpliced(...args);
    },
    unshift(...args) {
      return noTracking(this, "unshift", args);
    },
    values() {
      return iterator(this, "values", (item) => toWrapped(this, item));
    }
  };
  function iterator(self2, method, wrapValue) {
    const arr = shallowReadArray(self2);
    const iter = arr[method]();
    if (arr !== self2 && !/* @__PURE__ */ isShallow(self2)) {
      iter._next = iter.next;
      iter.next = () => {
        const result = iter._next();
        if (!result.done) {
          result.value = wrapValue(result.value);
        }
        return result;
      };
    }
    return iter;
  }
  const arrayProto = Array.prototype;
  function apply(self2, method, fn, thisArg, wrappedRetFn, args) {
    const arr = shallowReadArray(self2);
    const needsWrap = arr !== self2 && !/* @__PURE__ */ isShallow(self2);
    const methodFn = arr[method];
    if (methodFn !== arrayProto[method]) {
      const result2 = methodFn.apply(self2, args);
      return needsWrap ? toReactive(result2) : result2;
    }
    let wrappedFn = fn;
    if (arr !== self2) {
      if (needsWrap) {
        wrappedFn = function(item, index) {
          return fn.call(this, toWrapped(self2, item), index, self2);
        };
      } else if (fn.length > 2) {
        wrappedFn = function(item, index) {
          return fn.call(this, item, index, self2);
        };
      }
    }
    const result = methodFn.call(arr, wrappedFn, thisArg);
    return needsWrap && wrappedRetFn ? wrappedRetFn(result) : result;
  }
  function reduce(self2, method, fn, args) {
    const arr = shallowReadArray(self2);
    const needsWrap = arr !== self2 && !/* @__PURE__ */ isShallow(self2);
    let wrappedFn = fn;
    let wrapInitialAccumulator = false;
    if (arr !== self2) {
      if (needsWrap) {
        wrapInitialAccumulator = args.length === 0;
        wrappedFn = function(acc, item, index) {
          if (wrapInitialAccumulator) {
            wrapInitialAccumulator = false;
            acc = toWrapped(self2, acc);
          }
          return fn.call(this, acc, toWrapped(self2, item), index, self2);
        };
      } else if (fn.length > 3) {
        wrappedFn = function(acc, item, index) {
          return fn.call(this, acc, item, index, self2);
        };
      }
    }
    const result = arr[method](wrappedFn, ...args);
    return wrapInitialAccumulator ? toWrapped(self2, result) : result;
  }
  function searchProxy(self2, method, args) {
    const arr = /* @__PURE__ */ toRaw(self2);
    track(arr, "iterate", ARRAY_ITERATE_KEY);
    const res = arr[method](...args);
    if ((res === -1 || res === false) && /* @__PURE__ */ isProxy(args[0])) {
      args[0] = /* @__PURE__ */ toRaw(args[0]);
      return arr[method](...args);
    }
    return res;
  }
  function noTracking(self2, method, args = []) {
    pauseTracking();
    startBatch();
    const res = (/* @__PURE__ */ toRaw(self2))[method].apply(self2, args);
    endBatch();
    resetTracking();
    return res;
  }
  const isNonTrackableKeys = /* @__PURE__ */ makeMap(`__proto__,__v_isRef,__isVue`);
  const builtInSymbols = new Set(
    /* @__PURE__ */ Object.getOwnPropertyNames(Symbol).filter((key) => key !== "arguments" && key !== "caller").map((key) => Symbol[key]).filter(isSymbol)
  );
  function hasOwnProperty(key) {
    if (!isSymbol(key)) key = String(key);
    const obj = /* @__PURE__ */ toRaw(this);
    track(obj, "has", key);
    return obj.hasOwnProperty(key);
  }
  class BaseReactiveHandler {
    constructor(_isReadonly = false, _isShallow = false) {
      this._isReadonly = _isReadonly;
      this._isShallow = _isShallow;
    }
    get(target, key, receiver) {
      if (key === "__v_skip") return target["__v_skip"];
      const isReadonly2 = this._isReadonly, isShallow2 = this._isShallow;
      if (key === "__v_isReactive") {
        return !isReadonly2;
      } else if (key === "__v_isReadonly") {
        return isReadonly2;
      } else if (key === "__v_isShallow") {
        return isShallow2;
      } else if (key === "__v_raw") {
        if (receiver === (isReadonly2 ? isShallow2 ? shallowReadonlyMap : readonlyMap : isShallow2 ? shallowReactiveMap : reactiveMap).get(target) || // receiver is not the reactive proxy, but has the same prototype
        // this means the receiver is a user proxy of the reactive proxy
        Object.getPrototypeOf(target) === Object.getPrototypeOf(receiver)) {
          return target;
        }
        return;
      }
      const targetIsArray = isArray(target);
      if (!isReadonly2) {
        let fn;
        if (targetIsArray && (fn = arrayInstrumentations[key])) {
          return fn;
        }
        if (key === "hasOwnProperty") {
          return hasOwnProperty;
        }
      }
      const res = Reflect.get(
        target,
        key,
        // if this is a proxy wrapping a ref, return methods using the raw ref
        // as receiver so that we don't have to call `toRaw` on the ref in all
        // its class methods
        /* @__PURE__ */ isRef(target) ? target : receiver
      );
      if (isSymbol(key) ? builtInSymbols.has(key) : isNonTrackableKeys(key)) {
        return res;
      }
      if (!isReadonly2) {
        track(target, "get", key);
      }
      if (isShallow2) {
        return res;
      }
      if (/* @__PURE__ */ isRef(res)) {
        const value = targetIsArray && isIntegerKey(key) ? res : res.value;
        return isReadonly2 && isObject(value) ? /* @__PURE__ */ readonly(value) : value;
      }
      if (isObject(res)) {
        return isReadonly2 ? /* @__PURE__ */ readonly(res) : /* @__PURE__ */ reactive(res);
      }
      return res;
    }
  }
  class MutableReactiveHandler extends BaseReactiveHandler {
    constructor(isShallow2 = false) {
      super(false, isShallow2);
    }
    set(target, key, value, receiver) {
      let oldValue = target[key];
      const isArrayWithIntegerKey = isArray(target) && isIntegerKey(key);
      if (!this._isShallow) {
        const isOldValueReadonly = /* @__PURE__ */ isReadonly(oldValue);
        if (!/* @__PURE__ */ isShallow(value) && !/* @__PURE__ */ isReadonly(value)) {
          oldValue = /* @__PURE__ */ toRaw(oldValue);
          value = /* @__PURE__ */ toRaw(value);
        }
        if (!isArrayWithIntegerKey && /* @__PURE__ */ isRef(oldValue) && !/* @__PURE__ */ isRef(value)) {
          if (isOldValueReadonly) {
            return true;
          } else {
            oldValue.value = value;
            return true;
          }
        }
      }
      const hadKey = isArrayWithIntegerKey ? Number(key) < target.length : hasOwn(target, key);
      const result = Reflect.set(
        target,
        key,
        value,
        /* @__PURE__ */ isRef(target) ? target : receiver
      );
      if (target === /* @__PURE__ */ toRaw(receiver) && result) {
        if (!hadKey) {
          trigger(target, "add", key, value);
        } else if (hasChanged(value, oldValue)) {
          trigger(target, "set", key, value);
        }
      }
      return result;
    }
    deleteProperty(target, key) {
      const hadKey = hasOwn(target, key);
      target[key];
      const result = Reflect.deleteProperty(target, key);
      if (result && hadKey) {
        trigger(target, "delete", key, void 0);
      }
      return result;
    }
    has(target, key) {
      const result = Reflect.has(target, key);
      if (!isSymbol(key) || !builtInSymbols.has(key)) {
        track(target, "has", key);
      }
      return result;
    }
    ownKeys(target) {
      track(
        target,
        "iterate",
        isArray(target) ? "length" : ITERATE_KEY
      );
      return Reflect.ownKeys(target);
    }
  }
  class ReadonlyReactiveHandler extends BaseReactiveHandler {
    constructor(isShallow2 = false) {
      super(true, isShallow2);
    }
    set(target, key) {
      return true;
    }
    deleteProperty(target, key) {
      return true;
    }
  }
  const mutableHandlers = /* @__PURE__ */ new MutableReactiveHandler();
  const readonlyHandlers = /* @__PURE__ */ new ReadonlyReactiveHandler();
  const shallowReactiveHandlers = /* @__PURE__ */ new MutableReactiveHandler(true);
  const shallowReadonlyHandlers = /* @__PURE__ */ new ReadonlyReactiveHandler(true);
  const toShallow = (value) => value;
  const getProto = (v) => Reflect.getPrototypeOf(v);
  function createIterableMethod(method, isReadonly2, isShallow2) {
    return function(...args) {
      const target = this["__v_raw"];
      const rawTarget = /* @__PURE__ */ toRaw(target);
      const targetIsMap = isMap(rawTarget);
      const isPair = method === "entries" || method === Symbol.iterator && targetIsMap;
      const isKeyOnly = method === "keys" && targetIsMap;
      const innerIterator = target[method](...args);
      const wrap = isShallow2 ? toShallow : isReadonly2 ? toReadonly : toReactive;
      !isReadonly2 && track(
        rawTarget,
        "iterate",
        isKeyOnly ? MAP_KEY_ITERATE_KEY : ITERATE_KEY
      );
      return extend(
        // inheriting all iterator properties
        Object.create(innerIterator),
        {
          // iterator protocol
          next() {
            const { value, done } = innerIterator.next();
            return done ? { value, done } : {
              value: isPair ? [wrap(value[0]), wrap(value[1])] : wrap(value),
              done
            };
          }
        }
      );
    };
  }
  function createReadonlyMethod(type) {
    return function(...args) {
      return type === "delete" ? false : type === "clear" ? void 0 : this;
    };
  }
  function createInstrumentations(readonly2, shallow) {
    const instrumentations = {
      get(key) {
        const target = this["__v_raw"];
        const rawTarget = /* @__PURE__ */ toRaw(target);
        const rawKey = /* @__PURE__ */ toRaw(key);
        if (!readonly2) {
          if (hasChanged(key, rawKey)) {
            track(rawTarget, "get", key);
          }
          track(rawTarget, "get", rawKey);
        }
        const { has } = getProto(rawTarget);
        const wrap = shallow ? toShallow : readonly2 ? toReadonly : toReactive;
        if (has.call(rawTarget, key)) {
          return wrap(target.get(key));
        } else if (has.call(rawTarget, rawKey)) {
          return wrap(target.get(rawKey));
        } else if (target !== rawTarget) {
          target.get(key);
        }
      },
      get size() {
        const target = this["__v_raw"];
        !readonly2 && track(/* @__PURE__ */ toRaw(target), "iterate", ITERATE_KEY);
        return target.size;
      },
      has(key) {
        const target = this["__v_raw"];
        const rawTarget = /* @__PURE__ */ toRaw(target);
        const rawKey = /* @__PURE__ */ toRaw(key);
        if (!readonly2) {
          if (hasChanged(key, rawKey)) {
            track(rawTarget, "has", key);
          }
          track(rawTarget, "has", rawKey);
        }
        return key === rawKey ? target.has(key) : target.has(key) || target.has(rawKey);
      },
      forEach(callback, thisArg) {
        const observed = this;
        const target = observed["__v_raw"];
        const rawTarget = /* @__PURE__ */ toRaw(target);
        const wrap = shallow ? toShallow : readonly2 ? toReadonly : toReactive;
        !readonly2 && track(rawTarget, "iterate", ITERATE_KEY);
        return target.forEach((value, key) => {
          return callback.call(thisArg, wrap(value), wrap(key), observed);
        });
      }
    };
    extend(
      instrumentations,
      readonly2 ? {
        add: createReadonlyMethod("add"),
        set: createReadonlyMethod("set"),
        delete: createReadonlyMethod("delete"),
        clear: createReadonlyMethod("clear")
      } : {
        add(value) {
          const target = /* @__PURE__ */ toRaw(this);
          const proto = getProto(target);
          const rawValue = /* @__PURE__ */ toRaw(value);
          const valueToAdd = !shallow && !/* @__PURE__ */ isShallow(value) && !/* @__PURE__ */ isReadonly(value) ? rawValue : value;
          const hadKey = proto.has.call(target, valueToAdd) || hasChanged(value, valueToAdd) && proto.has.call(target, value) || hasChanged(rawValue, valueToAdd) && proto.has.call(target, rawValue);
          if (!hadKey) {
            target.add(valueToAdd);
            trigger(target, "add", valueToAdd, valueToAdd);
          }
          return this;
        },
        set(key, value) {
          if (!shallow && !/* @__PURE__ */ isShallow(value) && !/* @__PURE__ */ isReadonly(value)) {
            value = /* @__PURE__ */ toRaw(value);
          }
          const target = /* @__PURE__ */ toRaw(this);
          const { has, get } = getProto(target);
          let hadKey = has.call(target, key);
          if (!hadKey) {
            key = /* @__PURE__ */ toRaw(key);
            hadKey = has.call(target, key);
          }
          const oldValue = get.call(target, key);
          target.set(key, value);
          if (!hadKey) {
            trigger(target, "add", key, value);
          } else if (hasChanged(value, oldValue)) {
            trigger(target, "set", key, value);
          }
          return this;
        },
        delete(key) {
          const target = /* @__PURE__ */ toRaw(this);
          const { has, get } = getProto(target);
          let hadKey = has.call(target, key);
          if (!hadKey) {
            key = /* @__PURE__ */ toRaw(key);
            hadKey = has.call(target, key);
          }
          get ? get.call(target, key) : void 0;
          const result = target.delete(key);
          if (hadKey) {
            trigger(target, "delete", key, void 0);
          }
          return result;
        },
        clear() {
          const target = /* @__PURE__ */ toRaw(this);
          const hadItems = target.size !== 0;
          const result = target.clear();
          if (hadItems) {
            trigger(
              target,
              "clear",
              void 0,
              void 0
            );
          }
          return result;
        }
      }
    );
    const iteratorMethods = [
      "keys",
      "values",
      "entries",
      Symbol.iterator
    ];
    iteratorMethods.forEach((method) => {
      instrumentations[method] = createIterableMethod(method, readonly2, shallow);
    });
    return instrumentations;
  }
  function createInstrumentationGetter(isReadonly2, shallow) {
    const instrumentations = createInstrumentations(isReadonly2, shallow);
    return (target, key, receiver) => {
      if (key === "__v_isReactive") {
        return !isReadonly2;
      } else if (key === "__v_isReadonly") {
        return isReadonly2;
      } else if (key === "__v_raw") {
        return target;
      }
      return Reflect.get(
        hasOwn(instrumentations, key) && key in target ? instrumentations : target,
        key,
        receiver
      );
    };
  }
  const mutableCollectionHandlers = {
    get: /* @__PURE__ */ createInstrumentationGetter(false, false)
  };
  const shallowCollectionHandlers = {
    get: /* @__PURE__ */ createInstrumentationGetter(false, true)
  };
  const readonlyCollectionHandlers = {
    get: /* @__PURE__ */ createInstrumentationGetter(true, false)
  };
  const shallowReadonlyCollectionHandlers = {
    get: /* @__PURE__ */ createInstrumentationGetter(true, true)
  };
  const reactiveMap = /* @__PURE__ */ new WeakMap();
  const shallowReactiveMap = /* @__PURE__ */ new WeakMap();
  const readonlyMap = /* @__PURE__ */ new WeakMap();
  const shallowReadonlyMap = /* @__PURE__ */ new WeakMap();
  function targetTypeMap(rawType) {
    switch (rawType) {
      case "Object":
      case "Array":
        return 1;
      case "Map":
      case "Set":
      case "WeakMap":
      case "WeakSet":
        return 2;
      default:
        return 0;
    }
  }
  // @__NO_SIDE_EFFECTS__
  function reactive(target) {
    if (/* @__PURE__ */ isReadonly(target)) {
      return target;
    }
    return createReactiveObject(
      target,
      false,
      mutableHandlers,
      mutableCollectionHandlers,
      reactiveMap
    );
  }
  // @__NO_SIDE_EFFECTS__
  function shallowReactive(target) {
    return createReactiveObject(
      target,
      false,
      shallowReactiveHandlers,
      shallowCollectionHandlers,
      shallowReactiveMap
    );
  }
  // @__NO_SIDE_EFFECTS__
  function readonly(target) {
    return createReactiveObject(
      target,
      true,
      readonlyHandlers,
      readonlyCollectionHandlers,
      readonlyMap
    );
  }
  // @__NO_SIDE_EFFECTS__
  function shallowReadonly(target) {
    return createReactiveObject(
      target,
      true,
      shallowReadonlyHandlers,
      shallowReadonlyCollectionHandlers,
      shallowReadonlyMap
    );
  }
  function createReactiveObject(target, isReadonly2, baseHandlers, collectionHandlers, proxyMap) {
    if (!isObject(target)) {
      return target;
    }
    if (target["__v_raw"] && !(isReadonly2 && target["__v_isReactive"])) {
      return target;
    }
    if (target["__v_skip"] || !Object.isExtensible(target)) {
      return target;
    }
    const existingProxy = proxyMap.get(target);
    if (existingProxy) {
      return existingProxy;
    }
    const targetType = targetTypeMap(toRawType(target));
    if (targetType === 0) {
      return target;
    }
    const proxy = new Proxy(
      target,
      targetType === 2 ? collectionHandlers : baseHandlers
    );
    proxyMap.set(target, proxy);
    return proxy;
  }
  // @__NO_SIDE_EFFECTS__
  function isReactive(value) {
    if (/* @__PURE__ */ isReadonly(value)) {
      return /* @__PURE__ */ isReactive(value["__v_raw"]);
    }
    return !!(value && value["__v_isReactive"]);
  }
  // @__NO_SIDE_EFFECTS__
  function isReadonly(value) {
    return !!(value && value["__v_isReadonly"]);
  }
  // @__NO_SIDE_EFFECTS__
  function isShallow(value) {
    return !!(value && value["__v_isShallow"]);
  }
  // @__NO_SIDE_EFFECTS__
  function isProxy(value) {
    return value ? !!value["__v_raw"] : false;
  }
  // @__NO_SIDE_EFFECTS__
  function toRaw(observed) {
    const raw = observed && observed["__v_raw"];
    return raw ? /* @__PURE__ */ toRaw(raw) : observed;
  }
  function markRaw(value) {
    if (!hasOwn(value, "__v_skip") && Object.isExtensible(value)) {
      def(value, "__v_skip", true);
    }
    return value;
  }
  const toReactive = (value) => isObject(value) ? /* @__PURE__ */ reactive(value) : value;
  const toReadonly = (value) => isObject(value) ? /* @__PURE__ */ readonly(value) : value;
  // @__NO_SIDE_EFFECTS__
  function isRef(r) {
    return r ? r["__v_isRef"] === true : false;
  }
  // @__NO_SIDE_EFFECTS__
  function ref(value) {
    return createRef(value, false);
  }
  function createRef(rawValue, shallow) {
    if (/* @__PURE__ */ isRef(rawValue)) {
      return rawValue;
    }
    return new RefImpl(rawValue, shallow);
  }
  class RefImpl {
    constructor(value, isShallow2) {
      this.dep = new Dep();
      this["__v_isRef"] = true;
      this["__v_isShallow"] = false;
      this._rawValue = isShallow2 ? value : /* @__PURE__ */ toRaw(value);
      this._value = isShallow2 ? value : toReactive(value);
      this["__v_isShallow"] = isShallow2;
    }
    get value() {
      {
        this.dep.track();
      }
      return this._value;
    }
    set value(newValue) {
      const oldValue = this._rawValue;
      const useDirectValue = this["__v_isShallow"] || /* @__PURE__ */ isShallow(newValue) || /* @__PURE__ */ isReadonly(newValue);
      newValue = useDirectValue ? newValue : /* @__PURE__ */ toRaw(newValue);
      if (hasChanged(newValue, oldValue)) {
        this._rawValue = newValue;
        this._value = useDirectValue ? newValue : toReactive(newValue);
        {
          this.dep.trigger();
        }
      }
    }
  }
  function unref(ref2) {
    return /* @__PURE__ */ isRef(ref2) ? ref2.value : ref2;
  }
  const shallowUnwrapHandlers = {
    get: (target, key, receiver) => key === "__v_raw" ? target : unref(Reflect.get(target, key, receiver)),
    set: (target, key, value, receiver) => {
      const oldValue = target[key];
      if (/* @__PURE__ */ isRef(oldValue) && !/* @__PURE__ */ isRef(value)) {
        oldValue.value = value;
        return true;
      } else {
        return Reflect.set(target, key, value, receiver);
      }
    }
  };
  function proxyRefs(objectWithRefs) {
    return /* @__PURE__ */ isReactive(objectWithRefs) ? objectWithRefs : new Proxy(objectWithRefs, shallowUnwrapHandlers);
  }
  class ComputedRefImpl {
    constructor(fn, setter, isSSR) {
      this.fn = fn;
      this.setter = setter;
      this._value = void 0;
      this.dep = new Dep(this);
      this.__v_isRef = true;
      this.deps = void 0;
      this.depsTail = void 0;
      this.flags = 16;
      this.globalVersion = globalVersion - 1;
      this.next = void 0;
      this.effect = this;
      this["__v_isReadonly"] = !setter;
      this.isSSR = isSSR;
    }
    /**
     * @internal
     */
    notify() {
      this.flags |= 16;
      if (!(this.flags & 8) && // avoid infinite self recursion
      activeSub !== this) {
        batch(this, true);
        return true;
      }
    }
    get value() {
      const link = this.dep.track();
      refreshComputed(this);
      if (link) {
        link.version = this.dep.version;
      }
      return this._value;
    }
    set value(newValue) {
      if (this.setter) {
        this.setter(newValue);
      }
    }
  }
  // @__NO_SIDE_EFFECTS__
  function computed$1(getterOrOptions, debugOptions, isSSR = false) {
    let getter;
    let setter;
    if (isFunction(getterOrOptions)) {
      getter = getterOrOptions;
    } else {
      getter = getterOrOptions.get;
      setter = getterOrOptions.set;
    }
    const cRef = new ComputedRefImpl(getter, setter, isSSR);
    return cRef;
  }
  const INITIAL_WATCHER_VALUE = {};
  const cleanupMap = /* @__PURE__ */ new WeakMap();
  let activeWatcher = void 0;
  function onWatcherCleanup(cleanupFn, failSilently = false, owner = activeWatcher) {
    if (owner) {
      let cleanups = cleanupMap.get(owner);
      if (!cleanups) cleanupMap.set(owner, cleanups = []);
      cleanups.push(cleanupFn);
    }
  }
  function watch$1(source, cb, options = EMPTY_OBJ) {
    const { immediate, deep, once, scheduler, augmentJob, call } = options;
    const reactiveGetter = (source2) => {
      if (deep) return source2;
      if (/* @__PURE__ */ isShallow(source2) || deep === false || deep === 0)
        return traverse(source2, 1);
      return traverse(source2);
    };
    let effect2;
    let getter;
    let cleanup;
    let boundCleanup;
    let forceTrigger = false;
    let isMultiSource = false;
    if (/* @__PURE__ */ isRef(source)) {
      getter = () => source.value;
      forceTrigger = /* @__PURE__ */ isShallow(source);
    } else if (/* @__PURE__ */ isReactive(source)) {
      getter = () => reactiveGetter(source);
      forceTrigger = true;
    } else if (isArray(source)) {
      isMultiSource = true;
      forceTrigger = source.some((s) => /* @__PURE__ */ isReactive(s) || /* @__PURE__ */ isShallow(s));
      getter = () => source.map((s) => {
        if (/* @__PURE__ */ isRef(s)) {
          return s.value;
        } else if (/* @__PURE__ */ isReactive(s)) {
          return reactiveGetter(s);
        } else if (isFunction(s)) {
          return call ? call(s, 2) : s();
        } else ;
      });
    } else if (isFunction(source)) {
      if (cb) {
        getter = call ? () => call(source, 2) : source;
      } else {
        getter = () => {
          if (cleanup) {
            pauseTracking();
            try {
              cleanup();
            } finally {
              resetTracking();
            }
          }
          const currentEffect = activeWatcher;
          activeWatcher = effect2;
          try {
            return call ? call(source, 3, [boundCleanup]) : source(boundCleanup);
          } finally {
            activeWatcher = currentEffect;
          }
        };
      }
    } else {
      getter = NOOP;
    }
    if (cb && deep) {
      const baseGetter = getter;
      const depth = deep === true ? Infinity : deep;
      getter = () => traverse(baseGetter(), depth);
    }
    const scope = getCurrentScope();
    const watchHandle = () => {
      effect2.stop();
      if (scope && scope.active) {
        remove(scope.effects, effect2);
      }
    };
    if (once && cb) {
      const _cb = cb;
      cb = (...args) => {
        const res = _cb(...args);
        watchHandle();
        return res;
      };
    }
    let oldValue = isMultiSource ? new Array(source.length).fill(INITIAL_WATCHER_VALUE) : INITIAL_WATCHER_VALUE;
    const job = (immediateFirstRun) => {
      if (!(effect2.flags & 1) || !effect2.dirty && !immediateFirstRun) {
        return;
      }
      if (cb) {
        const newValue = effect2.run();
        if (immediateFirstRun || deep || forceTrigger || (isMultiSource ? newValue.some((v, i) => hasChanged(v, oldValue[i])) : hasChanged(newValue, oldValue))) {
          if (cleanup) {
            cleanup();
          }
          const currentWatcher = activeWatcher;
          activeWatcher = effect2;
          try {
            const args = [
              newValue,
              // pass undefined as the old value when it's changed for the first time
              oldValue === INITIAL_WATCHER_VALUE ? void 0 : isMultiSource && oldValue[0] === INITIAL_WATCHER_VALUE ? [] : oldValue,
              boundCleanup
            ];
            oldValue = newValue;
            call ? call(cb, 3, args) : (
              // @ts-expect-error
              cb(...args)
            );
          } finally {
            activeWatcher = currentWatcher;
          }
        }
      } else {
        effect2.run();
      }
    };
    if (augmentJob) {
      augmentJob(job);
    }
    effect2 = new ReactiveEffect(getter);
    effect2.scheduler = scheduler ? () => scheduler(job, false) : job;
    boundCleanup = (fn) => onWatcherCleanup(fn, false, effect2);
    cleanup = effect2.onStop = () => {
      const cleanups = cleanupMap.get(effect2);
      if (cleanups) {
        if (call) {
          call(cleanups, 4);
        } else {
          for (const cleanup2 of cleanups) cleanup2();
        }
        cleanupMap.delete(effect2);
      }
    };
    if (cb) {
      if (immediate) {
        job(true);
      } else {
        oldValue = effect2.run();
      }
    } else if (scheduler) {
      scheduler(job.bind(null, true), true);
    } else {
      effect2.run();
    }
    watchHandle.pause = effect2.pause.bind(effect2);
    watchHandle.resume = effect2.resume.bind(effect2);
    watchHandle.stop = watchHandle;
    return watchHandle;
  }
  function traverse(value, depth = Infinity, seen) {
    if (depth <= 0 || !isObject(value) || value["__v_skip"]) {
      return value;
    }
    seen = seen || /* @__PURE__ */ new Map();
    if ((seen.get(value) || 0) >= depth) {
      return value;
    }
    seen.set(value, depth);
    depth--;
    if (/* @__PURE__ */ isRef(value)) {
      traverse(value.value, depth, seen);
    } else if (isArray(value)) {
      for (let i = 0; i < value.length; i++) {
        traverse(value[i], depth, seen);
      }
    } else if (isSet(value) || isMap(value)) {
      value.forEach((v) => {
        traverse(v, depth, seen);
      });
    } else if (isPlainObject(value)) {
      for (const key in value) {
        traverse(value[key], depth, seen);
      }
      for (const key of Object.getOwnPropertySymbols(value)) {
        if (Object.prototype.propertyIsEnumerable.call(value, key)) {
          traverse(value[key], depth, seen);
        }
      }
    }
    return value;
  }
  /**
  * @vue/runtime-core v3.5.42
  * (c) 2018-present Yuxi (Evan) You and Vue contributors
  * @license MIT
  **/
  const stack = [];
  let isWarning = false;
  function warn$1(msg, ...args) {
    if (isWarning) return;
    isWarning = true;
    pauseTracking();
    const instance = stack.length ? stack[stack.length - 1].component : null;
    const appWarnHandler = instance && instance.appContext.config.warnHandler;
    const trace = getComponentTrace();
    if (appWarnHandler) {
      callWithErrorHandling(
        appWarnHandler,
        instance,
        11,
        [
          // eslint-disable-next-line no-restricted-syntax
          msg + args.map((a) => {
            var _a, _b;
            return (_b = (_a = a.toString) == null ? void 0 : _a.call(a)) != null ? _b : JSON.stringify(a);
          }).join(""),
          instance && instance.proxy,
          trace.map(
            ({ vnode }) => `at <${formatComponentName(instance, vnode.type)}>`
          ).join("\n"),
          trace
        ]
      );
    } else {
      const warnArgs = [`[Vue warn]: ${msg}`, ...args];
      if (trace.length && // avoid spamming console during tests
      true) {
        warnArgs.push(`
`, ...formatTrace(trace));
      }
      console.warn(...warnArgs);
    }
    resetTracking();
    isWarning = false;
  }
  function getComponentTrace() {
    let currentVNode = stack[stack.length - 1];
    if (!currentVNode) {
      return [];
    }
    const normalizedStack = [];
    while (currentVNode) {
      const last = normalizedStack[0];
      if (last && last.vnode === currentVNode) {
        last.recurseCount++;
      } else {
        normalizedStack.push({
          vnode: currentVNode,
          recurseCount: 0
        });
      }
      const parentInstance = currentVNode.component && currentVNode.component.parent;
      currentVNode = parentInstance && parentInstance.vnode;
    }
    return normalizedStack;
  }
  function formatTrace(trace) {
    const logs = [];
    trace.forEach((entry, i) => {
      logs.push(...i === 0 ? [] : [`
`], ...formatTraceEntry(entry));
    });
    return logs;
  }
  function formatTraceEntry({ vnode, recurseCount }) {
    const postfix = recurseCount > 0 ? `... (${recurseCount} recursive calls)` : ``;
    const isRoot = vnode.component ? vnode.component.parent == null : false;
    const open = ` at <${formatComponentName(
      vnode.component,
      vnode.type,
      isRoot
    )}`;
    const close = `>` + postfix;
    return vnode.props ? [open, ...formatProps(vnode.props), close] : [open + close];
  }
  function formatProps(props) {
    const res = [];
    const keys = Object.keys(props);
    keys.slice(0, 3).forEach((key) => {
      res.push(...formatProp(key, props[key]));
    });
    if (keys.length > 3) {
      res.push(` ...`);
    }
    return res;
  }
  function formatProp(key, value, raw) {
    if (isString(value)) {
      value = JSON.stringify(value);
      return raw ? value : [`${key}=${value}`];
    } else if (typeof value === "number" || typeof value === "boolean" || value == null) {
      return raw ? value : [`${key}=${value}`];
    } else if (/* @__PURE__ */ isRef(value)) {
      value = formatProp(key, /* @__PURE__ */ toRaw(value.value), true);
      return raw ? value : [`${key}=Ref<`, value, `>`];
    } else if (isFunction(value)) {
      return [`${key}=fn${value.name ? `<${value.name}>` : ``}`];
    } else {
      value = /* @__PURE__ */ toRaw(value);
      return raw ? value : [`${key}=`, value];
    }
  }
  function callWithErrorHandling(fn, instance, type, args) {
    try {
      return args ? fn(...args) : fn();
    } catch (err) {
      handleError(err, instance, type);
    }
  }
  function callWithAsyncErrorHandling(fn, instance, type, args) {
    if (isFunction(fn)) {
      const res = callWithErrorHandling(fn, instance, type, args);
      if (res && isPromise(res)) {
        res.catch((err) => {
          handleError(err, instance, type);
        });
      }
      return res;
    }
    if (isArray(fn)) {
      const values = [];
      for (let i = 0; i < fn.length; i++) {
        values.push(callWithAsyncErrorHandling(fn[i], instance, type, args));
      }
      return values;
    }
  }
  function handleError(err, instance, type, throwInDev = true) {
    const contextVNode = instance ? instance.vnode : null;
    const { errorHandler, throwUnhandledErrorInProduction } = instance && instance.appContext.config || EMPTY_OBJ;
    if (instance) {
      let cur = instance.parent;
      const exposedInstance = instance.proxy;
      const errorInfo = `https://vuejs.org/error-reference/#runtime-${type}`;
      while (cur) {
        const errorCapturedHooks = cur.ec;
        if (errorCapturedHooks) {
          for (let i = 0; i < errorCapturedHooks.length; i++) {
            if (errorCapturedHooks[i](err, exposedInstance, errorInfo) === false) {
              return;
            }
          }
        }
        cur = cur.parent;
      }
      if (errorHandler) {
        pauseTracking();
        callWithErrorHandling(errorHandler, null, 10, [
          err,
          exposedInstance,
          errorInfo
        ]);
        resetTracking();
        return;
      }
    }
    logError(err, type, contextVNode, throwInDev, throwUnhandledErrorInProduction);
  }
  function logError(err, type, contextVNode, throwInDev = true, throwInProd = false) {
    if (throwInProd) {
      throw err;
    } else {
      console.error(err);
    }
  }
  const queue = [];
  let flushIndex = -1;
  const pendingPostFlushCbs = [];
  let activePostFlushCbs = null;
  let postFlushIndex = 0;
  const resolvedPromise = /* @__PURE__ */ Promise.resolve();
  let currentFlushPromise = null;
  function nextTick(fn) {
    const p2 = currentFlushPromise || resolvedPromise;
    return fn ? p2.then(this ? fn.bind(this) : fn) : p2;
  }
  function findInsertionIndex(id) {
    let start = flushIndex + 1;
    let end = queue.length;
    while (start < end) {
      const middle = start + end >>> 1;
      const middleJob = queue[middle];
      const middleJobId = getId(middleJob);
      if (middleJobId < id || middleJobId === id && middleJob.flags & 2) {
        start = middle + 1;
      } else {
        end = middle;
      }
    }
    return start;
  }
  function queueJob(job) {
    if (!(job.flags & 1)) {
      const jobId = getId(job);
      const lastJob = queue[queue.length - 1];
      if (!lastJob || // fast path when the job id is larger than the tail
      !(job.flags & 2) && jobId >= getId(lastJob)) {
        queue.push(job);
      } else {
        queue.splice(findInsertionIndex(jobId), 0, job);
      }
      job.flags |= 1;
      queueFlush();
    }
  }
  function queueFlush() {
    if (!currentFlushPromise) {
      currentFlushPromise = resolvedPromise.then(flushJobs);
    }
  }
  function queuePostFlushCb(cb) {
    if (!isArray(cb)) {
      if (activePostFlushCbs && cb.id === -1) {
        activePostFlushCbs.splice(postFlushIndex + 1, 0, cb);
      } else if (!(cb.flags & 1)) {
        pendingPostFlushCbs.push(cb);
        cb.flags |= 1;
      }
    } else {
      for (let i = 0; i < cb.length; i++) {
        pendingPostFlushCbs.push(cb[i]);
      }
    }
    queueFlush();
  }
  function flushPreFlushCbs(instance, seen, i = flushIndex + 1) {
    for (; i < queue.length; i++) {
      const cb = queue[i];
      if (cb && cb.flags & 2) {
        if (instance && cb.id !== instance.uid) {
          continue;
        }
        queue.splice(i, 1);
        i--;
        if (cb.flags & 4) {
          cb.flags &= -2;
        }
        cb();
        if (!(cb.flags & 4)) {
          cb.flags &= -2;
        }
      }
    }
  }
  function flushPostFlushCbs(seen) {
    if (pendingPostFlushCbs.length) {
      const deduped = [...new Set(pendingPostFlushCbs)].sort(
        (a, b) => getId(a) - getId(b)
      );
      pendingPostFlushCbs.length = 0;
      if (activePostFlushCbs) {
        for (let i = 0; i < deduped.length; i++) {
          activePostFlushCbs.push(deduped[i]);
        }
        return;
      }
      activePostFlushCbs = deduped;
      for (postFlushIndex = 0; postFlushIndex < activePostFlushCbs.length; postFlushIndex++) {
        const cb = activePostFlushCbs[postFlushIndex];
        if (cb.flags & 4) {
          cb.flags &= -2;
        }
        if (!(cb.flags & 8)) cb();
        cb.flags &= -2;
      }
      activePostFlushCbs = null;
      postFlushIndex = 0;
    }
  }
  const getId = (job) => job.id == null ? job.flags & 2 ? -1 : Infinity : job.id;
  function flushJobs(seen) {
    try {
      for (flushIndex = 0; flushIndex < queue.length; flushIndex++) {
        const job = queue[flushIndex];
        if (job && !(job.flags & 8)) {
          if (false) ;
          if (job.flags & 4) {
            job.flags &= ~1;
          }
          callWithErrorHandling(
            job,
            job.i,
            job.i ? 15 : 14
          );
          if (!(job.flags & 4)) {
            job.flags &= ~1;
          }
        }
      }
    } finally {
      for (; flushIndex < queue.length; flushIndex++) {
        const job = queue[flushIndex];
        if (job) {
          job.flags &= -2;
        }
      }
      flushIndex = -1;
      queue.length = 0;
      flushPostFlushCbs();
      currentFlushPromise = null;
      if (queue.length || pendingPostFlushCbs.length) {
        flushJobs();
      }
    }
  }
  let currentRenderingInstance = null;
  let currentScopeId = null;
  function setCurrentRenderingInstance(instance) {
    const prev = currentRenderingInstance;
    currentRenderingInstance = instance;
    currentScopeId = instance && instance.type.__scopeId || null;
    return prev;
  }
  function withCtx(fn, ctx = currentRenderingInstance, isNonScopedSlot) {
    if (!ctx) return fn;
    if (fn._n) {
      return fn;
    }
    const renderFnWithContext = (...args) => {
      if (renderFnWithContext._d) {
        setBlockTracking(-1);
      }
      const prevInstance = setCurrentRenderingInstance(ctx);
      const prevStackSize = blockStack.length;
      let res;
      try {
        res = fn(...args);
      } finally {
        for (let i = blockStack.length; i > prevStackSize; i--) closeBlock();
        setCurrentRenderingInstance(prevInstance);
        if (renderFnWithContext._d) {
          setBlockTracking(1);
        }
      }
      return res;
    };
    renderFnWithContext._n = true;
    renderFnWithContext._c = true;
    renderFnWithContext._d = true;
    return renderFnWithContext;
  }
  function withDirectives(vnode, directives) {
    if (currentRenderingInstance === null) {
      return vnode;
    }
    const instance = getComponentPublicInstance(currentRenderingInstance);
    const bindings = vnode.dirs || (vnode.dirs = []);
    for (let i = 0; i < directives.length; i++) {
      let [dir, value, arg, modifiers = EMPTY_OBJ] = directives[i];
      if (dir) {
        if (isFunction(dir)) {
          dir = {
            mounted: dir,
            updated: dir
          };
        }
        if (dir.deep) {
          traverse(value);
        }
        bindings.push({
          dir,
          instance,
          value,
          oldValue: void 0,
          arg,
          modifiers
        });
      }
    }
    return vnode;
  }
  function invokeDirectiveHook(vnode, prevVNode, instance, name) {
    const bindings = vnode.dirs;
    const oldBindings = prevVNode && prevVNode.dirs;
    for (let i = 0; i < bindings.length; i++) {
      const binding = bindings[i];
      if (oldBindings) {
        binding.oldValue = oldBindings[i].value;
      }
      let hook = binding.dir[name];
      if (hook) {
        pauseTracking();
        callWithAsyncErrorHandling(hook, instance, 8, [
          vnode.el,
          binding,
          vnode,
          prevVNode
        ]);
        resetTracking();
      }
    }
  }
  function provide(key, value) {
    if (currentInstance) {
      let provides = currentInstance.provides;
      const parentProvides = currentInstance.parent && currentInstance.parent.provides;
      if (parentProvides === provides) {
        provides = currentInstance.provides = Object.create(parentProvides);
      }
      provides[key] = value;
    }
  }
  function inject(key, defaultValue, treatDefaultAsFactory = false) {
    const instance = getCurrentInstance();
    if (instance || currentApp) {
      let provides = currentApp ? currentApp._context.provides : instance ? instance.parent == null || instance.ce ? instance.vnode.appContext && instance.vnode.appContext.provides : instance.parent.provides : void 0;
      if (provides && key in provides) {
        return provides[key];
      } else if (arguments.length > 1) {
        return treatDefaultAsFactory && isFunction(defaultValue) ? defaultValue.call(instance && instance.proxy) : defaultValue;
      } else ;
    }
  }
  const ssrContextKey = /* @__PURE__ */ Symbol.for("v-scx");
  const useSSRContext = () => {
    {
      const ctx = inject(ssrContextKey);
      return ctx;
    }
  };
  function watchEffect(effect2, options) {
    return doWatch(effect2, null, options);
  }
  function watch(source, cb, options) {
    return doWatch(source, cb, options);
  }
  function doWatch(source, cb, options = EMPTY_OBJ) {
    const { immediate, deep, flush, once } = options;
    const baseWatchOptions = extend({}, options);
    const runsImmediately = cb && immediate || !cb && flush !== "post";
    let ssrCleanup;
    if (isInSSRComponentSetup) {
      if (flush === "sync") {
        const ctx = useSSRContext();
        ssrCleanup = ctx.__watcherHandles || (ctx.__watcherHandles = []);
      } else if (!runsImmediately) {
        const watchStopHandle = () => {
        };
        watchStopHandle.stop = NOOP;
        watchStopHandle.resume = NOOP;
        watchStopHandle.pause = NOOP;
        return watchStopHandle;
      }
    }
    const instance = currentInstance;
    baseWatchOptions.call = (fn, type, args) => callWithAsyncErrorHandling(fn, instance, type, args);
    let isPre = false;
    if (flush === "post") {
      baseWatchOptions.scheduler = (job) => {
        queuePostRenderEffect(job, instance && instance.suspense);
      };
    } else if (flush !== "sync") {
      isPre = true;
      baseWatchOptions.scheduler = (job, isFirstRun) => {
        if (isFirstRun) {
          job();
        } else {
          queueJob(job);
        }
      };
    }
    baseWatchOptions.augmentJob = (job) => {
      if (cb) {
        job.flags |= 4;
      }
      if (isPre) {
        job.flags |= 2;
        if (instance) {
          job.id = instance.uid;
          job.i = instance;
        }
      }
    };
    const watchHandle = watch$1(source, cb, baseWatchOptions);
    if (isInSSRComponentSetup) {
      if (ssrCleanup) {
        ssrCleanup.push(watchHandle);
      } else if (runsImmediately) {
        watchHandle();
      }
    }
    return watchHandle;
  }
  function instanceWatch(source, value, options) {
    const publicThis = this.proxy;
    const getter = isString(source) ? source.includes(".") ? createPathGetter(publicThis, source) : () => publicThis[source] : source.bind(publicThis, publicThis);
    let cb;
    if (isFunction(value)) {
      cb = value;
    } else {
      cb = value.handler;
      options = value;
    }
    const reset = setCurrentInstance(this);
    const res = doWatch(getter, cb.bind(publicThis), options);
    reset();
    return res;
  }
  function createPathGetter(ctx, path) {
    const segments = path.split(".");
    return () => {
      let cur = ctx;
      for (let i = 0; i < segments.length && cur; i++) {
        cur = cur[segments[i]];
      }
      return cur;
    };
  }
  const TeleportEndKey = /* @__PURE__ */ Symbol("_vte");
  const isTeleport = (type) => type.__isTeleport;
  const leaveCbKey = /* @__PURE__ */ Symbol("_leaveCb");
  function findNonCommentChild(children) {
    let child = children[0];
    if (children.length > 1) {
      for (const c of children) {
        if (c.type !== Comment) {
          child = c;
          break;
        }
      }
    }
    return child;
  }
  function getInnerChild$1(vnode) {
    if (!isKeepAlive(vnode)) {
      if (isTeleport(vnode.type) && vnode.children) {
        return findNonCommentChild(vnode.children);
      }
      return vnode;
    }
    if (vnode.component) {
      return vnode.component.subTree;
    }
    const { shapeFlag, children } = vnode;
    if (children) {
      if (shapeFlag & 16) {
        return children[0];
      }
      if (shapeFlag & 32 && isFunction(children.default)) {
        return children.default();
      }
    }
  }
  function setTransitionHooks(vnode, hooks) {
    if (vnode.shapeFlag & 6 && vnode.component) {
      vnode.transition = hooks;
      const subTree = vnode.component.subTree;
      setTransitionHooks(
        isTeleport(subTree.type) ? getInnerChild$1(subTree) || subTree : subTree,
        hooks
      );
    } else if (vnode.shapeFlag & 128) {
      vnode.ssContent.transition = hooks.clone(vnode.ssContent);
      vnode.ssFallback.transition = hooks.clone(vnode.ssFallback);
    } else {
      vnode.transition = hooks;
    }
  }
  // @__NO_SIDE_EFFECTS__
  function defineComponent(options, extraOptions) {
    return isFunction(options) ? (
      // #8236: extend call and options.name access are considered side-effects
      // by Rollup, so we have to wrap it in a pure-annotated IIFE.
      /* @__PURE__ */ (() => extend({ name: options.name }, extraOptions, { setup: options }))()
    ) : options;
  }
  function markAsyncBoundary(instance) {
    instance.ids = [instance.ids[0] + instance.ids[2]++ + "-", 0, 0];
  }
  function isTemplateRefKey(refs, key) {
    let desc;
    return !!((desc = Object.getOwnPropertyDescriptor(refs, key)) && !desc.configurable);
  }
  const pendingSetRefMap = /* @__PURE__ */ new WeakMap();
  function setRef(rawRef, oldRawRef, parentSuspense, vnode, isUnmount = false) {
    if (isArray(rawRef)) {
      rawRef.forEach(
        (r, i) => setRef(
          r,
          oldRawRef && (isArray(oldRawRef) ? oldRawRef[i] : oldRawRef),
          parentSuspense,
          vnode,
          isUnmount
        )
      );
      return;
    }
    if (isAsyncWrapper(vnode) && !isUnmount) {
      if (vnode.shapeFlag & 512 && vnode.type.__asyncResolved && vnode.component.subTree.component) {
        setRef(rawRef, oldRawRef, parentSuspense, vnode.component.subTree);
      }
      return;
    }
    const refValue = vnode.shapeFlag & 4 ? getComponentPublicInstance(vnode.component) : vnode.el;
    const value = isUnmount ? null : refValue;
    const { i: owner, r: ref3 } = rawRef;
    const oldRef = oldRawRef && oldRawRef.r;
    const refs = owner.refs === EMPTY_OBJ ? owner.refs = {} : owner.refs;
    const setupState = owner.setupState;
    const rawSetupState = /* @__PURE__ */ toRaw(setupState);
    const canSetSetupRef = setupState === EMPTY_OBJ ? NO : (key) => {
      if (isTemplateRefKey(refs, key)) {
        return false;
      }
      return hasOwn(rawSetupState, key);
    };
    const canSetRef = (ref22, key) => {
      if (key && isTemplateRefKey(refs, key)) {
        return false;
      }
      return true;
    };
    if (oldRef != null && oldRef !== ref3) {
      invalidatePendingSetRef(oldRawRef);
      if (isString(oldRef)) {
        refs[oldRef] = null;
        if (canSetSetupRef(oldRef)) {
          setupState[oldRef] = null;
        }
      } else if (/* @__PURE__ */ isRef(oldRef)) {
        const oldRawRefAtom = oldRawRef;
        if (canSetRef(oldRef, oldRawRefAtom.k)) {
          oldRef.value = null;
        }
        if (oldRawRefAtom.k) refs[oldRawRefAtom.k] = null;
      }
    }
    if (isFunction(ref3)) {
      callWithErrorHandling(ref3, owner, 12, [value, refs]);
    } else {
      const _isString = isString(ref3);
      const _isRef = /* @__PURE__ */ isRef(ref3);
      if (_isString || _isRef) {
        const doSet = () => {
          if (rawRef.f) {
            const existing = _isString ? canSetSetupRef(ref3) ? setupState[ref3] : refs[ref3] : canSetRef() || !rawRef.k ? ref3.value : refs[rawRef.k];
            if (isUnmount) {
              isArray(existing) && remove(existing, refValue);
            } else {
              if (!isArray(existing)) {
                if (_isString) {
                  refs[ref3] = [refValue];
                  if (canSetSetupRef(ref3)) {
                    setupState[ref3] = refs[ref3];
                  }
                } else {
                  const newVal = [refValue];
                  if (canSetRef(ref3, rawRef.k)) {
                    ref3.value = newVal;
                  }
                  if (rawRef.k) refs[rawRef.k] = newVal;
                }
              } else if (!existing.includes(refValue)) {
                existing.push(refValue);
              }
            }
          } else if (_isString) {
            refs[ref3] = value;
            if (canSetSetupRef(ref3)) {
              setupState[ref3] = value;
            }
          } else if (_isRef) {
            if (canSetRef(ref3, rawRef.k)) {
              ref3.value = value;
            }
            if (rawRef.k) refs[rawRef.k] = value;
          } else ;
        };
        if (value) {
          const job = () => {
            doSet();
            pendingSetRefMap.delete(rawRef);
          };
          job.id = -1;
          pendingSetRefMap.set(rawRef, job);
          queuePostRenderEffect(job, parentSuspense);
        } else {
          invalidatePendingSetRef(rawRef);
          doSet();
        }
      }
    }
  }
  function invalidatePendingSetRef(rawRef) {
    const pendingSetRef = pendingSetRefMap.get(rawRef);
    if (pendingSetRef) {
      pendingSetRef.flags |= 8;
      pendingSetRefMap.delete(rawRef);
    }
  }
  getGlobalThis().requestIdleCallback || ((cb) => setTimeout(cb, 1));
  getGlobalThis().cancelIdleCallback || ((id) => clearTimeout(id));
  const isAsyncWrapper = (i) => !!i.type.__asyncLoader;
  const isKeepAlive = (vnode) => vnode.type.__isKeepAlive;
  function onActivated(hook, target) {
    registerKeepAliveHook(hook, "a", target);
  }
  function onDeactivated(hook, target) {
    registerKeepAliveHook(hook, "da", target);
  }
  function registerKeepAliveHook(hook, type, target = currentInstance) {
    const wrappedHook = hook.__wdc || (hook.__wdc = () => {
      let current = target;
      while (current) {
        if (current.isDeactivated) {
          return;
        }
        current = current.parent;
      }
      return hook();
    });
    injectHook(type, wrappedHook, target);
    if (target) {
      let current = target.parent;
      while (current && current.parent) {
        if (isKeepAlive(current.parent.vnode)) {
          injectToKeepAliveRoot(wrappedHook, type, target, current);
        }
        current = current.parent;
      }
    }
  }
  function injectToKeepAliveRoot(hook, type, target, keepAliveRoot) {
    const injected = injectHook(
      type,
      hook,
      keepAliveRoot,
      true
      /* prepend */
    );
    onUnmounted(() => {
      remove(keepAliveRoot[type], injected);
    }, target);
  }
  function injectHook(type, hook, target = currentInstance, prepend = false) {
    if (target) {
      const hooks = target[type] || (target[type] = []);
      const wrappedHook = hook.__weh || (hook.__weh = (...args) => {
        pauseTracking();
        const reset = setCurrentInstance(target);
        const res = callWithAsyncErrorHandling(hook, target, type, args);
        reset();
        resetTracking();
        return res;
      });
      if (prepend) {
        hooks.unshift(wrappedHook);
      } else {
        hooks.push(wrappedHook);
      }
      return wrappedHook;
    }
  }
  const createHook = (lifecycle) => (hook, target = currentInstance) => {
    if (!isInSSRComponentSetup || lifecycle === "sp") {
      injectHook(lifecycle, (...args) => hook(...args), target);
    }
  };
  const onBeforeMount = createHook("bm");
  const onMounted = createHook("m");
  const onBeforeUpdate = createHook(
    "bu"
  );
  const onUpdated = createHook("u");
  const onBeforeUnmount = createHook(
    "bum"
  );
  const onUnmounted = createHook("um");
  const onServerPrefetch = createHook(
    "sp"
  );
  const onRenderTriggered = createHook("rtg");
  const onRenderTracked = createHook("rtc");
  function onErrorCaptured(hook, target = currentInstance) {
    injectHook("ec", hook, target);
  }
  const COMPONENTS = "components";
  const NULL_DYNAMIC_COMPONENT = /* @__PURE__ */ Symbol.for("v-ndc");
  function resolveDynamicComponent(component) {
    if (isString(component)) {
      return resolveAsset(COMPONENTS, component, false) || component;
    } else {
      return component || NULL_DYNAMIC_COMPONENT;
    }
  }
  function resolveAsset(type, name, warnMissing = true, maybeSelfReference = false) {
    const instance = currentRenderingInstance || currentInstance;
    if (instance) {
      const Component = instance.type;
      {
        const selfName = getComponentName(
          Component,
          false
        );
        if (selfName && (selfName === name || selfName === camelize(name) || selfName === capitalize(camelize(name)))) {
          return Component;
        }
      }
      const res = (
        // local registration
        // check instance[type] first which is resolved for options API
        resolve(instance[type] || Component[type], name) || // global registration
        resolve(instance.appContext[type], name)
      );
      if (!res && maybeSelfReference) {
        return Component;
      }
      return res;
    }
  }
  function resolve(registry, name) {
    return registry && (registry[name] || registry[camelize(name)] || registry[capitalize(camelize(name))]);
  }
  function renderList(source, renderItem, cache, index) {
    let ret;
    const cached = cache;
    const sourceIsArray = isArray(source);
    if (sourceIsArray || isString(source)) {
      const sourceIsReactiveArray = sourceIsArray && /* @__PURE__ */ isReactive(source);
      let needsWrap = false;
      let isReadonlySource = false;
      if (sourceIsReactiveArray) {
        needsWrap = !/* @__PURE__ */ isShallow(source);
        isReadonlySource = /* @__PURE__ */ isReadonly(source);
        source = shallowReadArray(source);
      }
      ret = new Array(source.length);
      for (let i = 0, l = source.length; i < l; i++) {
        ret[i] = renderItem(
          needsWrap ? isReadonlySource ? toReadonly(toReactive(source[i])) : toReactive(source[i]) : source[i],
          i,
          void 0,
          cached
        );
      }
    } else if (typeof source === "number") {
      {
        ret = new Array(source);
        for (let i = 0; i < source; i++) {
          ret[i] = renderItem(i + 1, i, void 0, cached);
        }
      }
    } else if (isObject(source)) {
      if (source[Symbol.iterator]) {
        ret = Array.from(
          source,
          (item, i) => renderItem(item, i, void 0, cached)
        );
      } else {
        const keys = Object.keys(source);
        ret = new Array(keys.length);
        for (let i = 0, l = keys.length; i < l; i++) {
          const key = keys[i];
          ret[i] = renderItem(source[key], key, i, cached);
        }
      }
    } else {
      ret = [];
    }
    return ret;
  }
  function renderSlot(slots, name, props, fallback, noSlotted, branchKey) {
    if (props == null) props = {};
    if (currentRenderingInstance.ce || currentRenderingInstance.parent && isAsyncWrapper(currentRenderingInstance.parent) && currentRenderingInstance.parent.ce) {
      const slotProps = props;
      const hasProps = Object.keys(slotProps).length > 0;
      return openBlock(), createBlock(
        Fragment,
        null,
        [createVNode("slot", slotProps, fallback)],
        hasProps ? -2 : 64
      );
    }
    let slot = slots[name];
    if (slot && slot._c) {
      slot._d = false;
    }
    const prevStackSize = blockStack.length;
    openBlock();
    let rendered;
    try {
      const validSlotContent = slot && ensureValidVNode(slot(props));
      const slotKey = props.key || branchKey || // slot content array of a dynamic conditional slot may have a branch
      // key attached in the `createSlots` helper, respect that
      validSlotContent && validSlotContent.key;
      rendered = createBlock(
        Fragment,
        {
          key: (slotKey && !isSymbol(slotKey) ? slotKey : `_${name}`) + // #7256 force differentiate fallback content from actual content
          (!validSlotContent && fallback ? "_fb" : "")
        },
        validSlotContent || (fallback ? fallback() : []),
        validSlotContent && slots._ === 1 ? 64 : -2
      );
    } catch (err) {
      for (let i = blockStack.length; i > prevStackSize; i--) closeBlock();
      throw err;
    } finally {
      if (slot && slot._c) {
        slot._d = true;
      }
    }
    if (rendered.scopeId) {
      rendered.slotScopeIds = [rendered.scopeId + "-s"];
    }
    return rendered;
  }
  function ensureValidVNode(vnodes) {
    return vnodes.some((child) => {
      if (!isVNode(child)) return true;
      if (child.type === Comment) return false;
      if (child.type === Fragment && !ensureValidVNode(child.children))
        return false;
      return true;
    }) ? vnodes : null;
  }
  const getPublicInstance = (i) => {
    if (!i) return null;
    if (isStatefulComponent(i)) return getComponentPublicInstance(i);
    return getPublicInstance(i.parent);
  };
  const publicPropertiesMap = (
    // Move PURE marker to new line to workaround compiler discarding it
    // due to type annotation
    /* @__PURE__ */ extend(/* @__PURE__ */ Object.create(null), {
      $: (i) => i,
      $el: (i) => i.vnode.el,
      $data: (i) => i.data,
      $props: (i) => i.props,
      $attrs: (i) => i.attrs,
      $slots: (i) => i.slots,
      $refs: (i) => i.refs,
      $parent: (i) => getPublicInstance(i.parent),
      $root: (i) => getPublicInstance(i.root),
      $host: (i) => i.ce,
      $emit: (i) => i.emit,
      $options: (i) => resolveMergedOptions(i),
      $forceUpdate: (i) => i.f || (i.f = () => {
        queueJob(i.update);
      }),
      $nextTick: (i) => i.n || (i.n = nextTick.bind(i.proxy)),
      $watch: (i) => instanceWatch.bind(i)
    })
  );
  const hasSetupBinding = (state2, key) => state2 !== EMPTY_OBJ && !state2.__isScriptSetup && hasOwn(state2, key);
  const PublicInstanceProxyHandlers = {
    get({ _: instance }, key) {
      if (key === "__v_skip") {
        return true;
      }
      const { ctx, setupState, data, props, accessCache, type, appContext } = instance;
      if (key[0] !== "$") {
        const n = accessCache[key];
        if (n !== void 0) {
          switch (n) {
            case 1:
              return setupState[key];
            case 2:
              return data[key];
            case 4:
              return ctx[key];
            case 3:
              return props[key];
          }
        } else if (hasSetupBinding(setupState, key)) {
          accessCache[key] = 1;
          return setupState[key];
        } else if (data !== EMPTY_OBJ && hasOwn(data, key)) {
          accessCache[key] = 2;
          return data[key];
        } else if (hasOwn(props, key)) {
          accessCache[key] = 3;
          return props[key];
        } else if (ctx !== EMPTY_OBJ && hasOwn(ctx, key)) {
          accessCache[key] = 4;
          return ctx[key];
        } else if (shouldCacheAccess) {
          accessCache[key] = 0;
        }
      }
      const publicGetter = publicPropertiesMap[key];
      let cssModule, globalProperties;
      if (publicGetter) {
        if (key === "$attrs") {
          track(instance.attrs, "get", "");
        }
        return publicGetter(instance);
      } else if (
        // css module (injected by vue-loader)
        (cssModule = type.__cssModules) && (cssModule = cssModule[key])
      ) {
        return cssModule;
      } else if (ctx !== EMPTY_OBJ && hasOwn(ctx, key)) {
        accessCache[key] = 4;
        return ctx[key];
      } else if (
        // global properties
        globalProperties = appContext.config.globalProperties, hasOwn(globalProperties, key)
      ) {
        {
          return globalProperties[key];
        }
      } else ;
    },
    set({ _: instance }, key, value) {
      const { data, setupState, ctx } = instance;
      if (hasSetupBinding(setupState, key)) {
        setupState[key] = value;
        return true;
      } else if (data !== EMPTY_OBJ && hasOwn(data, key)) {
        data[key] = value;
        return true;
      } else if (hasOwn(instance.props, key)) {
        return false;
      }
      if (key[0] === "$" && key.slice(1) in instance) {
        return false;
      } else {
        {
          ctx[key] = value;
        }
      }
      return true;
    },
    has({
      _: { data, setupState, accessCache, ctx, appContext, props, type }
    }, key) {
      let cssModules;
      return !!(accessCache[key] || data !== EMPTY_OBJ && key[0] !== "$" && hasOwn(data, key) || hasSetupBinding(setupState, key) || hasOwn(props, key) || hasOwn(ctx, key) || hasOwn(publicPropertiesMap, key) || hasOwn(appContext.config.globalProperties, key) || (cssModules = type.__cssModules) && cssModules[key]);
    },
    defineProperty(target, key, descriptor) {
      if (descriptor.get != null) {
        target._.accessCache[key] = 0;
      } else if (hasOwn(descriptor, "value")) {
        this.set(target, key, descriptor.value, null);
      }
      return Reflect.defineProperty(target, key, descriptor);
    }
  };
  function useSlots() {
    return getContext().slots;
  }
  function getContext(calledFunctionName) {
    const i = getCurrentInstance();
    return i.setupContext || (i.setupContext = createSetupContext(i));
  }
  function normalizePropsOrEmits(props) {
    return isArray(props) ? props.reduce(
      (normalized, p2) => (normalized[p2] = null, normalized),
      {}
    ) : props;
  }
  let shouldCacheAccess = true;
  function applyOptions(instance) {
    const options = resolveMergedOptions(instance);
    const publicThis = instance.proxy;
    const ctx = instance.ctx;
    shouldCacheAccess = false;
    if (options.beforeCreate) {
      callHook(options.beforeCreate, instance, "bc");
    }
    const {
      // state
      data: dataOptions,
      computed: computedOptions,
      methods,
      watch: watchOptions,
      provide: provideOptions,
      inject: injectOptions,
      // lifecycle
      created,
      beforeMount,
      mounted,
      beforeUpdate,
      updated,
      activated,
      deactivated,
      beforeDestroy,
      beforeUnmount,
      destroyed,
      unmounted,
      render,
      renderTracked,
      renderTriggered,
      errorCaptured,
      serverPrefetch,
      // public API
      expose,
      inheritAttrs,
      // assets
      components,
      directives,
      filters
    } = options;
    const checkDuplicateProperties = null;
    if (injectOptions) {
      resolveInjections(injectOptions, ctx, checkDuplicateProperties);
    }
    if (methods) {
      for (const key in methods) {
        const methodHandler = methods[key];
        if (isFunction(methodHandler)) {
          {
            ctx[key] = methodHandler.bind(publicThis);
          }
        }
      }
    }
    if (dataOptions) {
      const data = dataOptions.call(publicThis, publicThis);
      if (!isObject(data)) ;
      else {
        instance.data = /* @__PURE__ */ reactive(data);
      }
    }
    shouldCacheAccess = true;
    if (computedOptions) {
      for (const key in computedOptions) {
        const opt = computedOptions[key];
        const get = isFunction(opt) ? opt.bind(publicThis, publicThis) : isFunction(opt.get) ? opt.get.bind(publicThis, publicThis) : NOOP;
        const set = !isFunction(opt) && isFunction(opt.set) ? opt.set.bind(publicThis) : NOOP;
        const c = computed({
          get,
          set
        });
        Object.defineProperty(ctx, key, {
          enumerable: true,
          configurable: true,
          get: () => c.value,
          set: (v) => c.value = v
        });
      }
    }
    if (watchOptions) {
      for (const key in watchOptions) {
        createWatcher(watchOptions[key], ctx, publicThis, key);
      }
    }
    if (provideOptions) {
      const provides = isFunction(provideOptions) ? provideOptions.call(publicThis) : provideOptions;
      Reflect.ownKeys(provides).forEach((key) => {
        provide(key, provides[key]);
      });
    }
    if (created) {
      callHook(created, instance, "c");
    }
    function registerLifecycleHook(register, hook) {
      if (isArray(hook)) {
        hook.forEach((_hook) => register(_hook.bind(publicThis)));
      } else if (hook) {
        register(hook.bind(publicThis));
      }
    }
    registerLifecycleHook(onBeforeMount, beforeMount);
    registerLifecycleHook(onMounted, mounted);
    registerLifecycleHook(onBeforeUpdate, beforeUpdate);
    registerLifecycleHook(onUpdated, updated);
    registerLifecycleHook(onActivated, activated);
    registerLifecycleHook(onDeactivated, deactivated);
    registerLifecycleHook(onErrorCaptured, errorCaptured);
    registerLifecycleHook(onRenderTracked, renderTracked);
    registerLifecycleHook(onRenderTriggered, renderTriggered);
    registerLifecycleHook(onBeforeUnmount, beforeUnmount);
    registerLifecycleHook(onUnmounted, unmounted);
    registerLifecycleHook(onServerPrefetch, serverPrefetch);
    if (isArray(expose)) {
      if (expose.length) {
        const exposed = instance.exposed || (instance.exposed = {});
        expose.forEach((key) => {
          Object.defineProperty(exposed, key, {
            get: () => publicThis[key],
            set: (val) => publicThis[key] = val,
            enumerable: true
          });
        });
      } else if (!instance.exposed) {
        instance.exposed = {};
      }
    }
    if (render && instance.render === NOOP) {
      instance.render = render;
    }
    if (inheritAttrs != null) {
      instance.inheritAttrs = inheritAttrs;
    }
    if (components) instance.components = components;
    if (directives) instance.directives = directives;
    if (serverPrefetch) {
      markAsyncBoundary(instance);
    }
  }
  function resolveInjections(injectOptions, ctx, checkDuplicateProperties = NOOP) {
    if (isArray(injectOptions)) {
      injectOptions = normalizeInject(injectOptions);
    }
    for (const key in injectOptions) {
      const opt = injectOptions[key];
      let injected;
      if (isObject(opt)) {
        if ("default" in opt) {
          injected = inject(
            opt.from || key,
            opt.default,
            true
          );
        } else {
          injected = inject(opt.from || key);
        }
      } else {
        injected = inject(opt);
      }
      if (/* @__PURE__ */ isRef(injected)) {
        Object.defineProperty(ctx, key, {
          enumerable: true,
          configurable: true,
          get: () => injected.value,
          set: (v) => injected.value = v
        });
      } else {
        ctx[key] = injected;
      }
    }
  }
  function callHook(hook, instance, type) {
    callWithAsyncErrorHandling(
      isArray(hook) ? hook.map((h2) => h2.bind(instance.proxy)) : hook.bind(instance.proxy),
      instance,
      type
    );
  }
  function createWatcher(raw, ctx, publicThis, key) {
    let getter = key.includes(".") ? createPathGetter(publicThis, key) : () => publicThis[key];
    if (isString(raw)) {
      const handler = ctx[raw];
      if (isFunction(handler)) {
        {
          watch(getter, handler);
        }
      }
    } else if (isFunction(raw)) {
      {
        watch(getter, raw.bind(publicThis));
      }
    } else if (isObject(raw)) {
      if (isArray(raw)) {
        raw.forEach((r) => createWatcher(r, ctx, publicThis, key));
      } else {
        const handler = isFunction(raw.handler) ? raw.handler.bind(publicThis) : ctx[raw.handler];
        if (isFunction(handler)) {
          watch(getter, handler, raw);
        }
      }
    } else ;
  }
  function resolveMergedOptions(instance) {
    const base = instance.type;
    const { mixins, extends: extendsOptions } = base;
    const {
      mixins: globalMixins,
      optionsCache: cache,
      config: { optionMergeStrategies }
    } = instance.appContext;
    const cached = cache.get(base);
    let resolved;
    if (cached) {
      resolved = cached;
    } else if (!globalMixins.length && !mixins && !extendsOptions) {
      {
        resolved = base;
      }
    } else {
      resolved = {};
      if (globalMixins.length) {
        globalMixins.forEach(
          (m) => mergeOptions(resolved, m, optionMergeStrategies, true)
        );
      }
      mergeOptions(resolved, base, optionMergeStrategies);
    }
    if (isObject(base)) {
      cache.set(base, resolved);
    }
    return resolved;
  }
  function mergeOptions(to, from, strats, asMixin = false) {
    const { mixins, extends: extendsOptions } = from;
    if (extendsOptions) {
      mergeOptions(to, extendsOptions, strats, true);
    }
    if (mixins) {
      mixins.forEach(
        (m) => mergeOptions(to, m, strats, true)
      );
    }
    for (const key in from) {
      if (asMixin && key === "expose") ;
      else {
        const strat = internalOptionMergeStrats[key] || strats && strats[key];
        to[key] = strat ? strat(to[key], from[key]) : from[key];
      }
    }
    return to;
  }
  const internalOptionMergeStrats = {
    data: mergeDataFn,
    props: mergeEmitsOrPropsOptions,
    emits: mergeEmitsOrPropsOptions,
    // objects
    methods: mergeObjectOptions,
    computed: mergeObjectOptions,
    // lifecycle
    beforeCreate: mergeAsArray,
    created: mergeAsArray,
    beforeMount: mergeAsArray,
    mounted: mergeAsArray,
    beforeUpdate: mergeAsArray,
    updated: mergeAsArray,
    beforeDestroy: mergeAsArray,
    beforeUnmount: mergeAsArray,
    destroyed: mergeAsArray,
    unmounted: mergeAsArray,
    activated: mergeAsArray,
    deactivated: mergeAsArray,
    errorCaptured: mergeAsArray,
    serverPrefetch: mergeAsArray,
    // assets
    components: mergeObjectOptions,
    directives: mergeObjectOptions,
    // watch
    watch: mergeWatchOptions,
    // provide / inject
    provide: mergeDataFn,
    inject: mergeInject
  };
  function mergeDataFn(to, from) {
    if (!from) {
      return to;
    }
    if (!to) {
      return from;
    }
    return function mergedDataFn() {
      return extend(
        isFunction(to) ? to.call(this, this) : to,
        isFunction(from) ? from.call(this, this) : from
      );
    };
  }
  function mergeInject(to, from) {
    return mergeObjectOptions(normalizeInject(to), normalizeInject(from));
  }
  function normalizeInject(raw) {
    if (isArray(raw)) {
      const res = {};
      for (let i = 0; i < raw.length; i++) {
        res[raw[i]] = raw[i];
      }
      return res;
    }
    return raw;
  }
  function mergeAsArray(to, from) {
    return to ? [...new Set([].concat(to, from))] : from;
  }
  function mergeObjectOptions(to, from) {
    return to ? extend(/* @__PURE__ */ Object.create(null), to, from) : from;
  }
  function mergeEmitsOrPropsOptions(to, from) {
    if (to) {
      if (isArray(to) && isArray(from)) {
        return [.../* @__PURE__ */ new Set([...to, ...from])];
      }
      return extend(
        /* @__PURE__ */ Object.create(null),
        normalizePropsOrEmits(to),
        normalizePropsOrEmits(from != null ? from : {})
      );
    } else {
      return from;
    }
  }
  function mergeWatchOptions(to, from) {
    if (!to) return from;
    if (!from) return to;
    const merged = extend(/* @__PURE__ */ Object.create(null), to);
    for (const key in from) {
      merged[key] = mergeAsArray(to[key], from[key]);
    }
    return merged;
  }
  function createAppContext() {
    return {
      app: null,
      config: {
        isNativeTag: NO,
        performance: false,
        globalProperties: {},
        optionMergeStrategies: {},
        errorHandler: void 0,
        warnHandler: void 0,
        compilerOptions: {}
      },
      mixins: [],
      components: {},
      directives: {},
      provides: /* @__PURE__ */ Object.create(null),
      optionsCache: /* @__PURE__ */ new WeakMap(),
      propsCache: /* @__PURE__ */ new WeakMap(),
      emitsCache: /* @__PURE__ */ new WeakMap()
    };
  }
  let uid$1 = 0;
  function createAppAPI(render, hydrate) {
    return function createApp2(rootComponent, rootProps = null) {
      if (!isFunction(rootComponent)) {
        rootComponent = extend({}, rootComponent);
      }
      if (rootProps != null && !isObject(rootProps)) {
        rootProps = null;
      }
      const context = createAppContext();
      const installedPlugins = /* @__PURE__ */ new WeakSet();
      const pluginCleanupFns = [];
      let isMounted = false;
      const app = context.app = {
        _uid: uid$1++,
        _component: rootComponent,
        _props: rootProps,
        _container: null,
        _context: context,
        _instance: null,
        version,
        get config() {
          return context.config;
        },
        set config(v) {
        },
        use(plugin, ...options) {
          if (installedPlugins.has(plugin)) ;
          else if (plugin && isFunction(plugin.install)) {
            installedPlugins.add(plugin);
            plugin.install(app, ...options);
          } else if (isFunction(plugin)) {
            installedPlugins.add(plugin);
            plugin(app, ...options);
          } else ;
          return app;
        },
        mixin(mixin) {
          {
            if (!context.mixins.includes(mixin)) {
              context.mixins.push(mixin);
            }
          }
          return app;
        },
        component(name, component) {
          if (!component) {
            return context.components[name];
          }
          context.components[name] = component;
          return app;
        },
        directive(name, directive) {
          if (!directive) {
            return context.directives[name];
          }
          context.directives[name] = directive;
          return app;
        },
        mount(rootContainer, isHydrate, namespace) {
          if (!isMounted) {
            const vnode = app._ceVNode || createVNode(rootComponent, rootProps);
            vnode.appContext = context;
            if (namespace === true) {
              namespace = "svg";
            } else if (namespace === false) {
              namespace = void 0;
            }
            {
              render(vnode, rootContainer, namespace);
            }
            isMounted = true;
            app._container = rootContainer;
            rootContainer.__vue_app__ = app;
            return getComponentPublicInstance(vnode.component);
          }
        },
        onUnmount(cleanupFn) {
          pluginCleanupFns.push(cleanupFn);
        },
        unmount() {
          if (isMounted) {
            callWithAsyncErrorHandling(
              pluginCleanupFns,
              app._instance,
              16
            );
            render(null, app._container);
            delete app._container.__vue_app__;
          }
        },
        provide(key, value) {
          context.provides[key] = value;
          return app;
        },
        runWithContext(fn) {
          const lastApp = currentApp;
          currentApp = app;
          try {
            return fn();
          } finally {
            currentApp = lastApp;
          }
        }
      };
      return app;
    };
  }
  let currentApp = null;
  const getModelModifiers = (props, modelName) => {
    return modelName === "modelValue" || modelName === "model-value" ? props.modelModifiers : props[`${modelName}Modifiers`] || props[`${camelize(modelName)}Modifiers`] || props[`${hyphenate(modelName)}Modifiers`];
  };
  function emit(instance, event, ...rawArgs) {
    if (instance.isUnmounted) return;
    const props = instance.vnode.props || EMPTY_OBJ;
    let args = rawArgs;
    const isModelListener2 = event.startsWith("update:");
    const modifiers = isModelListener2 && getModelModifiers(props, event.slice(7));
    if (modifiers) {
      if (modifiers.trim) {
        args = rawArgs.map((a) => isString(a) ? a.trim() : a);
      }
      if (modifiers.number) {
        args = args.map(looseToNumber);
      }
    }
    let handlerName;
    let handler = props[handlerName = toHandlerKey(event)] || // also try camelCase event handler (#2249)
    props[handlerName = toHandlerKey(camelize(event))];
    if (!handler && isModelListener2) {
      handler = props[handlerName = toHandlerKey(hyphenate(event))];
    }
    if (handler) {
      callWithAsyncErrorHandling(
        handler,
        instance,
        6,
        args
      );
    }
    const onceHandler = props[handlerName + `Once`];
    if (onceHandler) {
      if (!instance.emitted) {
        instance.emitted = {};
      } else if (instance.emitted[handlerName]) {
        return;
      }
      instance.emitted[handlerName] = true;
      callWithAsyncErrorHandling(
        onceHandler,
        instance,
        6,
        args
      );
    }
  }
  const mixinEmitsCache = /* @__PURE__ */ new WeakMap();
  function normalizeEmitsOptions(comp, appContext, asMixin = false) {
    const cache = asMixin ? mixinEmitsCache : appContext.emitsCache;
    const cached = cache.get(comp);
    if (cached !== void 0) {
      return cached;
    }
    const raw = comp.emits;
    let normalized = {};
    let hasExtends = false;
    if (!isFunction(comp)) {
      const extendEmits = (raw2) => {
        const normalizedFromExtend = normalizeEmitsOptions(raw2, appContext, true);
        if (normalizedFromExtend) {
          hasExtends = true;
          extend(normalized, normalizedFromExtend);
        }
      };
      if (!asMixin && appContext.mixins.length) {
        appContext.mixins.forEach(extendEmits);
      }
      if (comp.extends) {
        extendEmits(comp.extends);
      }
      if (comp.mixins) {
        comp.mixins.forEach(extendEmits);
      }
    }
    if (!raw && !hasExtends) {
      if (isObject(comp)) {
        cache.set(comp, null);
      }
      return null;
    }
    if (isArray(raw)) {
      raw.forEach((key) => normalized[key] = null);
    } else {
      extend(normalized, raw);
    }
    if (isObject(comp)) {
      cache.set(comp, normalized);
    }
    return normalized;
  }
  function isEmitListener(options, key) {
    if (!options || !isOn(key)) {
      return false;
    }
    key = key.slice(2);
    key = key === "Once" ? key : key.replace(/Once$/, "");
    return hasOwn(options, key[0].toLowerCase() + key.slice(1)) || hasOwn(options, hyphenate(key)) || hasOwn(options, key);
  }
  function markAttrsAccessed() {
  }
  function renderComponentRoot(instance) {
    const {
      type: Component,
      vnode,
      proxy,
      withProxy,
      propsOptions: [propsOptions],
      slots,
      attrs,
      emit: emit2,
      render,
      renderCache,
      props,
      data,
      setupState,
      ctx,
      inheritAttrs
    } = instance;
    const prev = setCurrentRenderingInstance(instance);
    let result;
    let fallthroughAttrs;
    try {
      if (vnode.shapeFlag & 4) {
        const proxyToUse = withProxy || proxy;
        const thisProxy = false ? new Proxy(proxyToUse, {
          get(target, key, receiver) {
            warn$1(
              `Property '${String(
                key
              )}' was accessed via 'this'. Avoid using 'this' in templates.`
            );
            return Reflect.get(target, key, receiver);
          }
        }) : proxyToUse;
        result = normalizeVNode(
          render.call(
            thisProxy,
            proxyToUse,
            renderCache,
            false ? /* @__PURE__ */ shallowReadonly(props) : props,
            setupState,
            data,
            ctx
          )
        );
        fallthroughAttrs = attrs;
      } else {
        const render2 = Component;
        if (false) ;
        result = normalizeVNode(
          render2.length > 1 ? render2(
            false ? /* @__PURE__ */ shallowReadonly(props) : props,
            false ? {
              get attrs() {
                markAttrsAccessed();
                return /* @__PURE__ */ shallowReadonly(attrs);
              },
              slots,
              emit: emit2
            } : { attrs, slots, emit: emit2 }
          ) : render2(
            false ? /* @__PURE__ */ shallowReadonly(props) : props,
            null
          )
        );
        fallthroughAttrs = Component.props ? attrs : getFunctionalFallthrough(attrs);
      }
    } catch (err) {
      blockStack.length = 0;
      handleError(err, instance, 1);
      result = createVNode(Comment);
    }
    let root = result;
    if (fallthroughAttrs && inheritAttrs !== false) {
      const keys = Object.keys(fallthroughAttrs);
      const { shapeFlag } = root;
      if (keys.length) {
        if (shapeFlag & (1 | 6)) {
          if (propsOptions && keys.some(isModelListener)) {
            fallthroughAttrs = filterModelListeners(
              fallthroughAttrs,
              propsOptions
            );
          }
          root = cloneVNode(root, fallthroughAttrs, false, true);
        }
      }
    }
    if (vnode.dirs) {
      root = cloneVNode(root, null, false, true);
      root.dirs = root.dirs ? root.dirs.concat(vnode.dirs) : vnode.dirs;
    }
    if (vnode.transition) {
      const child = isTeleport(root.type) ? getInnerChild$1(root) || root : root;
      setTransitionHooks(child, vnode.transition);
    }
    {
      result = root;
    }
    setCurrentRenderingInstance(prev);
    return result;
  }
  const getFunctionalFallthrough = (attrs) => {
    let res;
    for (const key in attrs) {
      if (key === "class" || key === "style" || isOn(key)) {
        (res || (res = {}))[key] = attrs[key];
      }
    }
    return res;
  };
  const filterModelListeners = (attrs, props) => {
    const res = {};
    for (const key in attrs) {
      if (!isModelListener(key) || !(key.slice(9) in props)) {
        res[key] = attrs[key];
      }
    }
    return res;
  };
  function shouldUpdateComponent(prevVNode, nextVNode, optimized) {
    const { props: prevProps, children: prevChildren, component } = prevVNode;
    const { props: nextProps, children: nextChildren, patchFlag } = nextVNode;
    const emits = component.emitsOptions;
    if (nextVNode.dirs || nextVNode.transition) {
      return true;
    }
    if (optimized && patchFlag >= 0) {
      if (patchFlag & 1024) {
        return true;
      }
      if (patchFlag & 16) {
        if (!prevProps) {
          return !!nextProps;
        }
        return hasPropsChanged(prevProps, nextProps, emits);
      } else if (patchFlag & 8) {
        const dynamicProps = nextVNode.dynamicProps;
        for (let i = 0; i < dynamicProps.length; i++) {
          const key = dynamicProps[i];
          if (hasPropValueChanged(nextProps, prevProps, key) && !isEmitListener(emits, key)) {
            return true;
          }
        }
      }
    } else {
      if (prevChildren || nextChildren) {
        if (!nextChildren || !nextChildren.$stable) {
          return true;
        }
      }
      if (prevProps === nextProps) {
        return false;
      }
      if (!prevProps) {
        return !!nextProps;
      }
      if (!nextProps) {
        return true;
      }
      return hasPropsChanged(prevProps, nextProps, emits);
    }
    return false;
  }
  function hasPropsChanged(prevProps, nextProps, emitsOptions) {
    const nextKeys = Object.keys(nextProps);
    if (nextKeys.length !== Object.keys(prevProps).length) {
      return true;
    }
    for (let i = 0; i < nextKeys.length; i++) {
      const key = nextKeys[i];
      if (hasPropValueChanged(nextProps, prevProps, key) && !isEmitListener(emitsOptions, key)) {
        return true;
      }
    }
    return false;
  }
  function hasPropValueChanged(nextProps, prevProps, key) {
    const nextProp = nextProps[key];
    const prevProp = prevProps[key];
    if (key === "style" && isObject(nextProp) && isObject(prevProp)) {
      return !looseEqual(nextProp, prevProp);
    }
    return nextProp !== prevProp;
  }
  function updateHOCHostEl({ vnode, parent, suspense }, el2) {
    while (parent) {
      const root = parent.subTree;
      if (root.suspense && root.suspense.activeBranch === vnode) {
        root.suspense.vnode.el = root.el = el2;
        vnode = root;
      }
      if (root === vnode) {
        (vnode = parent.vnode).el = el2;
        parent = parent.parent;
      } else {
        break;
      }
    }
    if (suspense && suspense.activeBranch === vnode) {
      suspense.vnode.el = el2;
    }
  }
  const internalObjectProto = {};
  const createInternalObject = () => Object.create(internalObjectProto);
  const isInternalObject = (obj) => Object.getPrototypeOf(obj) === internalObjectProto;
  function initProps(instance, rawProps, isStateful, isSSR = false) {
    const props = {};
    const attrs = createInternalObject();
    instance.propsDefaults = /* @__PURE__ */ Object.create(null);
    setFullProps(instance, rawProps, props, attrs);
    for (const key in instance.propsOptions[0]) {
      if (!(key in props)) {
        props[key] = void 0;
      }
    }
    if (isStateful) {
      instance.props = isSSR ? props : /* @__PURE__ */ shallowReactive(props);
    } else {
      if (!instance.type.props) {
        instance.props = attrs;
      } else {
        instance.props = props;
      }
    }
    instance.attrs = attrs;
  }
  function updateProps(instance, rawProps, rawPrevProps, optimized) {
    const {
      props,
      attrs,
      vnode: { patchFlag }
    } = instance;
    const rawCurrentProps = /* @__PURE__ */ toRaw(props);
    const [options] = instance.propsOptions;
    let hasAttrsChanged = false;
    if (
      // always force full diff in dev
      // - #1942 if hmr is enabled with sfc component
      // - vite#872 non-sfc component used by sfc component
      (optimized || patchFlag > 0) && !(patchFlag & 16)
    ) {
      if (patchFlag & 8) {
        const propsToUpdate = instance.vnode.dynamicProps;
        for (let i = 0; i < propsToUpdate.length; i++) {
          let key = propsToUpdate[i];
          if (isEmitListener(instance.emitsOptions, key)) {
            continue;
          }
          const value = rawProps[key];
          if (options) {
            if (hasOwn(attrs, key)) {
              if (value !== attrs[key]) {
                attrs[key] = value;
                hasAttrsChanged = true;
              }
            } else {
              const camelizedKey = camelize(key);
              props[camelizedKey] = resolvePropValue(
                options,
                rawCurrentProps,
                camelizedKey,
                value,
                instance,
                false
              );
            }
          } else {
            if (value !== attrs[key]) {
              attrs[key] = value;
              hasAttrsChanged = true;
            }
          }
        }
      }
    } else {
      if (setFullProps(instance, rawProps, props, attrs)) {
        hasAttrsChanged = true;
      }
      let kebabKey;
      for (const key in rawCurrentProps) {
        if (!rawProps || // for camelCase
        !hasOwn(rawProps, key) && // it's possible the original props was passed in as kebab-case
        // and converted to camelCase (#955)
        ((kebabKey = hyphenate(key)) === key || !hasOwn(rawProps, kebabKey))) {
          if (options) {
            if (rawPrevProps && // for camelCase
            (rawPrevProps[key] !== void 0 || // for kebab-case
            rawPrevProps[kebabKey] !== void 0)) {
              props[key] = resolvePropValue(
                options,
                rawCurrentProps,
                key,
                void 0,
                instance,
                true
              );
            }
          } else {
            delete props[key];
          }
        }
      }
      if (attrs !== rawCurrentProps) {
        for (const key in attrs) {
          if (!rawProps || !hasOwn(rawProps, key) && true) {
            delete attrs[key];
            hasAttrsChanged = true;
          }
        }
      }
    }
    if (hasAttrsChanged) {
      trigger(instance.attrs, "set", "");
    }
  }
  function setFullProps(instance, rawProps, props, attrs) {
    const [options, needCastKeys] = instance.propsOptions;
    let hasAttrsChanged = false;
    let rawCastValues;
    if (rawProps) {
      for (let key in rawProps) {
        if (isReservedProp(key)) {
          continue;
        }
        const value = rawProps[key];
        let camelKey;
        if (options && hasOwn(options, camelKey = camelize(key))) {
          if (!needCastKeys || !needCastKeys.includes(camelKey)) {
            props[camelKey] = value;
          } else {
            (rawCastValues || (rawCastValues = {}))[camelKey] = value;
          }
        } else if (!isEmitListener(instance.emitsOptions, key)) {
          if (!(key in attrs) || value !== attrs[key]) {
            attrs[key] = value;
            hasAttrsChanged = true;
          }
        }
      }
    }
    if (needCastKeys) {
      const rawCurrentProps = /* @__PURE__ */ toRaw(props);
      const castValues = rawCastValues || EMPTY_OBJ;
      for (let i = 0; i < needCastKeys.length; i++) {
        const key = needCastKeys[i];
        props[key] = resolvePropValue(
          options,
          rawCurrentProps,
          key,
          castValues[key],
          instance,
          !hasOwn(castValues, key)
        );
      }
    }
    return hasAttrsChanged;
  }
  function resolvePropValue(options, props, key, value, instance, isAbsent) {
    const opt = options[key];
    if (opt != null) {
      const hasDefault = hasOwn(opt, "default");
      if (hasDefault && value === void 0) {
        const defaultValue = opt.default;
        if (opt.type !== Function && !opt.skipFactory && isFunction(defaultValue)) {
          const { propsDefaults } = instance;
          if (key in propsDefaults) {
            value = propsDefaults[key];
          } else {
            const reset = setCurrentInstance(instance);
            value = propsDefaults[key] = defaultValue.call(
              null,
              props
            );
            reset();
          }
        } else {
          value = defaultValue;
        }
        if (instance.ce) {
          instance.ce._setProp(key, value);
        }
      }
      if (opt[
        0
        /* shouldCast */
      ]) {
        if (isAbsent && !hasDefault) {
          value = false;
        } else if (opt[
          1
          /* shouldCastTrue */
        ] && (value === "" || value === hyphenate(key))) {
          value = true;
        }
      }
    }
    return value;
  }
  const mixinPropsCache = /* @__PURE__ */ new WeakMap();
  function normalizePropsOptions(comp, appContext, asMixin = false) {
    const cache = asMixin ? mixinPropsCache : appContext.propsCache;
    const cached = cache.get(comp);
    if (cached) {
      return cached;
    }
    const raw = comp.props;
    const normalized = {};
    const needCastKeys = [];
    let hasExtends = false;
    if (!isFunction(comp)) {
      const extendProps = (raw2) => {
        hasExtends = true;
        const [props, keys] = normalizePropsOptions(raw2, appContext, true);
        extend(normalized, props);
        if (keys) needCastKeys.push(...keys);
      };
      if (!asMixin && appContext.mixins.length) {
        appContext.mixins.forEach(extendProps);
      }
      if (comp.extends) {
        extendProps(comp.extends);
      }
      if (comp.mixins) {
        comp.mixins.forEach(extendProps);
      }
    }
    if (!raw && !hasExtends) {
      if (isObject(comp)) {
        cache.set(comp, EMPTY_ARR);
      }
      return EMPTY_ARR;
    }
    if (isArray(raw)) {
      for (let i = 0; i < raw.length; i++) {
        const normalizedKey = camelize(raw[i]);
        if (validatePropName(normalizedKey)) {
          normalized[normalizedKey] = EMPTY_OBJ;
        }
      }
    } else if (raw) {
      for (const key in raw) {
        const normalizedKey = camelize(key);
        if (validatePropName(normalizedKey)) {
          const opt = raw[key];
          const prop = normalized[normalizedKey] = isArray(opt) || isFunction(opt) ? { type: opt } : extend({}, opt);
          const propType = prop.type;
          let shouldCast = false;
          let shouldCastTrue = true;
          if (isArray(propType)) {
            for (let index = 0; index < propType.length; ++index) {
              const type = propType[index];
              const typeName = isFunction(type) && type.name;
              if (typeName === "Boolean") {
                shouldCast = true;
                break;
              } else if (typeName === "String") {
                shouldCastTrue = false;
              }
            }
          } else {
            shouldCast = isFunction(propType) && propType.name === "Boolean";
          }
          prop[
            0
            /* shouldCast */
          ] = shouldCast;
          prop[
            1
            /* shouldCastTrue */
          ] = shouldCastTrue;
          if (shouldCast || hasOwn(prop, "default")) {
            needCastKeys.push(normalizedKey);
          }
        }
      }
    }
    const res = [normalized, needCastKeys];
    if (isObject(comp)) {
      cache.set(comp, res);
    }
    return res;
  }
  function validatePropName(key) {
    if (key[0] !== "$" && !isReservedProp(key)) {
      return true;
    }
    return false;
  }
  const isInternalKey = (key) => key === "_" || key === "_ctx" || key === "$stable";
  const normalizeSlotValue = (value) => isArray(value) ? value.map(normalizeVNode) : [normalizeVNode(value)];
  const normalizeSlot = (key, rawSlot, ctx) => {
    if (rawSlot._n) {
      return rawSlot;
    }
    const normalized = withCtx((...args) => {
      if (false) ;
      return normalizeSlotValue(rawSlot(...args));
    }, ctx);
    normalized._c = false;
    return normalized;
  };
  const normalizeObjectSlots = (rawSlots, slots, instance) => {
    const ctx = rawSlots._ctx;
    for (const key in rawSlots) {
      if (isInternalKey(key)) continue;
      const value = rawSlots[key];
      if (isFunction(value)) {
        slots[key] = normalizeSlot(key, value, ctx);
      } else if (value != null) {
        const normalized = normalizeSlotValue(value);
        slots[key] = () => normalized;
      }
    }
  };
  const normalizeVNodeSlots = (instance, children) => {
    const normalized = normalizeSlotValue(children);
    instance.slots.default = () => normalized;
  };
  const assignSlots = (slots, children, optimized) => {
    for (const key in children) {
      if (optimized || !isInternalKey(key)) {
        slots[key] = children[key];
      }
    }
  };
  const initSlots = (instance, children, optimized) => {
    const slots = instance.slots = createInternalObject();
    if (instance.vnode.shapeFlag & 32) {
      const type = children._;
      if (type) {
        assignSlots(slots, children, optimized);
        if (optimized) {
          def(slots, "_", type, true);
        }
      } else {
        normalizeObjectSlots(children, slots);
      }
    } else if (children) {
      normalizeVNodeSlots(instance, children);
    }
  };
  const updateSlots = (instance, children, optimized) => {
    const { vnode, slots } = instance;
    let needDeletionCheck = true;
    let deletionComparisonTarget = EMPTY_OBJ;
    if (vnode.shapeFlag & 32) {
      const type = children._;
      if (type) {
        if (optimized && type === 1) {
          needDeletionCheck = false;
        } else {
          assignSlots(slots, children, optimized);
        }
      } else {
        needDeletionCheck = !children.$stable;
        normalizeObjectSlots(children, slots);
      }
      deletionComparisonTarget = children;
    } else if (children) {
      normalizeVNodeSlots(instance, children);
      deletionComparisonTarget = { default: 1 };
    }
    if (needDeletionCheck) {
      for (const key in slots) {
        if (!isInternalKey(key) && deletionComparisonTarget[key] == null) {
          delete slots[key];
        }
      }
    }
  };
  const queuePostRenderEffect = queueEffectWithSuspense;
  function createRenderer(options) {
    return baseCreateRenderer(options);
  }
  function baseCreateRenderer(options, createHydrationFns) {
    const target = getGlobalThis();
    target.__VUE__ = true;
    const {
      insert: hostInsert,
      remove: hostRemove,
      patchProp: hostPatchProp,
      createElement: hostCreateElement,
      createText: hostCreateText,
      createComment: hostCreateComment,
      setText: hostSetText,
      setElementText: hostSetElementText,
      parentNode: hostParentNode,
      nextSibling: hostNextSibling,
      setScopeId: hostSetScopeId = NOOP,
      insertStaticContent: hostInsertStaticContent
    } = options;
    const patch = (n1, n2, container, anchor = null, parentComponent = null, parentSuspense = null, namespace = void 0, slotScopeIds = null, optimized = !!n2.dynamicChildren) => {
      if (n1 === n2) {
        return;
      }
      if (n1 && !isSameVNodeType(n1, n2)) {
        anchor = getNextHostNode(n1);
        unmount(n1, parentComponent, parentSuspense, true);
        n1 = null;
      }
      if (n2.patchFlag === -2) {
        optimized = false;
        n2.dynamicChildren = null;
      }
      const { type, ref: ref3, shapeFlag } = n2;
      switch (type) {
        case Text:
          processText(n1, n2, container, anchor);
          break;
        case Comment:
          processCommentNode(n1, n2, container, anchor);
          break;
        case Static:
          if (n1 == null) {
            mountStaticNode(n2, container, anchor, namespace);
          }
          break;
        case Fragment:
          processFragment(
            n1,
            n2,
            container,
            anchor,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
          break;
        default:
          if (shapeFlag & 1) {
            processElement(
              n1,
              n2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
          } else if (shapeFlag & 6) {
            processComponent(
              n1,
              n2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
          } else if (shapeFlag & 64) {
            type.process(
              n1,
              n2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized,
              internals
            );
          } else if (shapeFlag & 128) {
            type.process(
              n1,
              n2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized,
              internals
            );
          } else ;
      }
      if (ref3 != null && parentComponent) {
        setRef(ref3, n1 && n1.ref, parentSuspense, n2 || n1, !n2);
      } else if (ref3 == null && n1 && n1.ref != null) {
        setRef(n1.ref, null, parentSuspense, n1, true);
      }
    };
    const processText = (n1, n2, container, anchor) => {
      if (n1 == null) {
        hostInsert(
          n2.el = hostCreateText(n2.children),
          container,
          anchor
        );
      } else {
        const el2 = n2.el = n1.el;
        if (n2.children !== n1.children) {
          hostSetText(el2, n2.children);
        }
      }
    };
    const processCommentNode = (n1, n2, container, anchor) => {
      if (n1 == null) {
        hostInsert(
          n2.el = hostCreateComment(n2.children || ""),
          container,
          anchor
        );
      } else {
        n2.el = n1.el;
      }
    };
    const mountStaticNode = (n2, container, anchor, namespace) => {
      [n2.el, n2.anchor] = hostInsertStaticContent(
        n2.children,
        container,
        anchor,
        namespace,
        n2.el,
        n2.anchor
      );
    };
    const moveStaticNode = ({ el: el2, anchor }, container, nextSibling) => {
      let next;
      while (el2 && el2 !== anchor) {
        next = hostNextSibling(el2);
        hostInsert(el2, container, nextSibling);
        el2 = next;
      }
      hostInsert(anchor, container, nextSibling);
    };
    const removeStaticNode = ({ el: el2, anchor }) => {
      let next;
      while (el2 && el2 !== anchor) {
        next = hostNextSibling(el2);
        hostRemove(el2);
        el2 = next;
      }
      hostRemove(anchor);
    };
    const processElement = (n1, n2, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      if (n2.type === "svg") {
        namespace = "svg";
      } else if (n2.type === "math") {
        namespace = "mathml";
      }
      if (n1 == null) {
        mountElement(
          n2,
          container,
          anchor,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          optimized
        );
      } else {
        const customElement = n1.el && n1.el._isVueCE ? n1.el : null;
        try {
          if (customElement) {
            customElement._beginPatch();
          }
          patchElement(
            n1,
            n2,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
        } finally {
          if (customElement) {
            customElement._endPatch();
          }
        }
      }
    };
    const mountElement = (vnode, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      let el2;
      let vnodeHook;
      const { props, shapeFlag, transition, dirs } = vnode;
      el2 = vnode.el = hostCreateElement(
        vnode.type,
        namespace,
        props && props.is,
        props
      );
      if (shapeFlag & 8) {
        hostSetElementText(el2, vnode.children);
      } else if (shapeFlag & 16) {
        mountChildren(
          vnode.children,
          el2,
          null,
          parentComponent,
          parentSuspense,
          resolveChildrenNamespace(vnode, namespace),
          slotScopeIds,
          optimized
        );
      }
      if (dirs) {
        invokeDirectiveHook(vnode, null, parentComponent, "created");
      }
      setScopeId(el2, vnode, vnode.scopeId, slotScopeIds, parentComponent);
      if (props) {
        for (const key in props) {
          if (key !== "value" && !isReservedProp(key)) {
            hostPatchProp(el2, key, null, props[key], namespace, parentComponent);
          }
        }
        if ("value" in props) {
          hostPatchProp(el2, "value", null, props.value, namespace);
        }
        if (vnodeHook = props.onVnodeBeforeMount) {
          invokeVNodeHook(vnodeHook, parentComponent, vnode);
        }
      }
      if (dirs) {
        invokeDirectiveHook(vnode, null, parentComponent, "beforeMount");
      }
      const needCallTransitionHooks = needTransition(parentSuspense, transition);
      if (needCallTransitionHooks) {
        transition.beforeEnter(el2);
      }
      hostInsert(el2, container, anchor);
      if ((vnodeHook = props && props.onVnodeMounted) || needCallTransitionHooks || dirs) {
        queuePostRenderEffect(() => {
          try {
            vnodeHook && invokeVNodeHook(vnodeHook, parentComponent, vnode);
            needCallTransitionHooks && transition.enter(el2);
            dirs && invokeDirectiveHook(vnode, null, parentComponent, "mounted");
          } finally {
          }
        }, parentSuspense);
      }
    };
    const setScopeId = (el2, vnode, scopeId, slotScopeIds, parentComponent) => {
      if (scopeId) {
        hostSetScopeId(el2, scopeId);
      }
      if (slotScopeIds) {
        for (let i = 0; i < slotScopeIds.length; i++) {
          hostSetScopeId(el2, slotScopeIds[i]);
        }
      }
      if (parentComponent) {
        let subTree = parentComponent.subTree;
        if (vnode === subTree || isSuspense(subTree.type) && (subTree.ssContent === vnode || subTree.ssFallback === vnode)) {
          const parentVNode = parentComponent.vnode;
          setScopeId(
            el2,
            parentVNode,
            parentVNode.scopeId,
            parentVNode.slotScopeIds,
            parentComponent.parent
          );
        }
      }
    };
    const mountChildren = (children, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized, start = 0) => {
      for (let i = start; i < children.length; i++) {
        const child = children[i] = optimized ? cloneIfMounted(children[i]) : normalizeVNode(children[i]);
        patch(
          null,
          child,
          container,
          anchor,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          optimized
        );
      }
    };
    const patchElement = (n1, n2, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      const el2 = n2.el = n1.el;
      let { patchFlag, dynamicChildren, dirs } = n2;
      patchFlag |= n1.patchFlag & 16;
      const oldProps = n1.props || EMPTY_OBJ;
      const newProps = n2.props || EMPTY_OBJ;
      let vnodeHook;
      parentComponent && toggleRecurse(parentComponent, false);
      if (vnodeHook = newProps.onVnodeBeforeUpdate) {
        invokeVNodeHook(vnodeHook, parentComponent, n2, n1);
      }
      if (dirs) {
        invokeDirectiveHook(n2, n1, parentComponent, "beforeUpdate");
      }
      parentComponent && toggleRecurse(parentComponent, true);
      if (
        // #6385 the old vnode may be a user-wrapped non-isomorphic block
        // Force full diff when block metadata is unstable.
        dynamicChildren && (!n1.dynamicChildren || n1.dynamicChildren.length !== dynamicChildren.length)
      ) {
        patchFlag = 0;
        optimized = false;
        dynamicChildren = null;
      }
      if (oldProps.innerHTML && newProps.innerHTML == null || oldProps.textContent && newProps.textContent == null) {
        hostSetElementText(el2, "");
      }
      if (dynamicChildren) {
        patchBlockChildren(
          n1.dynamicChildren,
          dynamicChildren,
          el2,
          parentComponent,
          parentSuspense,
          resolveChildrenNamespace(n2, namespace),
          slotScopeIds
        );
      } else if (!optimized) {
        patchChildren(
          n1,
          n2,
          el2,
          null,
          parentComponent,
          parentSuspense,
          resolveChildrenNamespace(n2, namespace),
          slotScopeIds,
          false
        );
      }
      if (patchFlag > 0) {
        if (patchFlag & 16) {
          patchProps(el2, oldProps, newProps, parentComponent, namespace);
        } else {
          if (patchFlag & 2) {
            if (oldProps.class !== newProps.class) {
              hostPatchProp(el2, "class", null, newProps.class, namespace);
            }
          }
          if (patchFlag & 4) {
            hostPatchProp(el2, "style", oldProps.style, newProps.style, namespace);
          }
          if (patchFlag & 8) {
            const propsToUpdate = n2.dynamicProps;
            for (let i = 0; i < propsToUpdate.length; i++) {
              const key = propsToUpdate[i];
              const prev = oldProps[key];
              const next = newProps[key];
              if (next !== prev || key === "value") {
                hostPatchProp(el2, key, prev, next, namespace, parentComponent);
              }
            }
          }
        }
        if (patchFlag & 1) {
          if (n1.children !== n2.children) {
            hostSetElementText(el2, n2.children);
          }
        }
      } else if (!optimized && dynamicChildren == null) {
        patchProps(el2, oldProps, newProps, parentComponent, namespace);
      }
      if ((vnodeHook = newProps.onVnodeUpdated) || dirs) {
        queuePostRenderEffect(() => {
          vnodeHook && invokeVNodeHook(vnodeHook, parentComponent, n2, n1);
          dirs && invokeDirectiveHook(n2, n1, parentComponent, "updated");
        }, parentSuspense);
      }
    };
    const patchBlockChildren = (oldChildren, newChildren, fallbackContainer, parentComponent, parentSuspense, namespace, slotScopeIds) => {
      for (let i = 0; i < newChildren.length; i++) {
        const oldVNode = oldChildren[i];
        const newVNode = newChildren[i];
        const container = (
          // oldVNode may be an errored async setup() component inside Suspense
          // which will not have a mounted element
          oldVNode.el && // - In the case of a Fragment, we need to provide the actual parent
          // of the Fragment itself so it can move its children.
          (oldVNode.type === Fragment || // - In the case of different nodes, there is going to be a replacement
          // which also requires the correct parent container
          !isSameVNodeType(oldVNode, newVNode) || // - In the case of a component, it could contain anything.
          oldVNode.shapeFlag & (6 | 64 | 128)) ? hostParentNode(oldVNode.el) : (
            // In other cases, the parent container is not actually used so we
            // just pass the block element here to avoid a DOM parentNode call.
            fallbackContainer
          )
        );
        patch(
          oldVNode,
          newVNode,
          container,
          null,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          true
        );
      }
    };
    const patchProps = (el2, oldProps, newProps, parentComponent, namespace) => {
      if (oldProps !== newProps) {
        if (oldProps !== EMPTY_OBJ) {
          for (const key in oldProps) {
            if (!isReservedProp(key) && !(key in newProps)) {
              hostPatchProp(
                el2,
                key,
                oldProps[key],
                null,
                namespace,
                parentComponent
              );
            }
          }
        }
        for (const key in newProps) {
          if (isReservedProp(key)) continue;
          const next = newProps[key];
          const prev = oldProps[key];
          if (next !== prev && key !== "value") {
            hostPatchProp(el2, key, prev, next, namespace, parentComponent);
          }
        }
        if ("value" in newProps) {
          hostPatchProp(el2, "value", oldProps.value, newProps.value, namespace);
        }
      }
    };
    const processFragment = (n1, n2, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      const fragmentStartAnchor = n2.el = n1 ? n1.el : hostCreateText("");
      const fragmentEndAnchor = n2.anchor = n1 ? n1.anchor : hostCreateText("");
      let { patchFlag, dynamicChildren, slotScopeIds: fragmentSlotScopeIds } = n2;
      if (fragmentSlotScopeIds) {
        slotScopeIds = slotScopeIds ? slotScopeIds.concat(fragmentSlotScopeIds) : fragmentSlotScopeIds;
      }
      if (n1 == null) {
        hostInsert(fragmentStartAnchor, container, anchor);
        hostInsert(fragmentEndAnchor, container, anchor);
        mountChildren(
          // #10007
          // such fragment like `<></>` will be compiled into
          // a fragment which doesn't have a children.
          // In this case fallback to an empty array
          n2.children || [],
          container,
          fragmentEndAnchor,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          optimized
        );
      } else {
        if (patchFlag > 0 && patchFlag & 64 && dynamicChildren && // #2715 the previous fragment could've been a BAILed one as a result
        // of renderSlot() with no valid children
        n1.dynamicChildren && n1.dynamicChildren.length === dynamicChildren.length) {
          patchBlockChildren(
            n1.dynamicChildren,
            dynamicChildren,
            container,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds
          );
          if (
            // #2080 if the stable fragment has a key, it's a <template v-for> that may
            //  get moved around. Make sure all root level vnodes inherit el.
            // #2134 or if it's a component root, it may also get moved around
            // as the component is being moved.
            n2.key != null || parentComponent && n2 === parentComponent.subTree
          ) {
            traverseStaticChildren(
              n1,
              n2,
              true
              /* shallow */
            );
          }
        } else {
          patchChildren(
            n1,
            n2,
            container,
            fragmentEndAnchor,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
        }
      }
    };
    const processComponent = (n1, n2, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      n2.slotScopeIds = slotScopeIds;
      if (n1 == null) {
        if (n2.shapeFlag & 512) {
          parentComponent.ctx.activate(
            n2,
            container,
            anchor,
            namespace,
            optimized
          );
        } else {
          mountComponent(
            n2,
            container,
            anchor,
            parentComponent,
            parentSuspense,
            namespace,
            optimized
          );
        }
      } else {
        updateComponent(n1, n2, optimized);
      }
    };
    const mountComponent = (initialVNode, container, anchor, parentComponent, parentSuspense, namespace, optimized) => {
      const instance = initialVNode.component = createComponentInstance(
        initialVNode,
        parentComponent,
        parentSuspense
      );
      if (isKeepAlive(initialVNode)) {
        instance.ctx.renderer = internals;
      }
      {
        setupComponent(instance, false, optimized);
      }
      if (instance.asyncDep) {
        parentSuspense && parentSuspense.registerDep(instance, setupRenderEffect, optimized);
        if (!initialVNode.el) {
          const placeholder = instance.subTree = createVNode(Comment);
          processCommentNode(null, placeholder, container, anchor);
          initialVNode.placeholder = placeholder.el;
        }
      } else {
        setupRenderEffect(
          instance,
          initialVNode,
          container,
          anchor,
          parentSuspense,
          namespace,
          optimized
        );
      }
    };
    const updateComponent = (n1, n2, optimized) => {
      const instance = n2.component = n1.component;
      if (shouldUpdateComponent(n1, n2, optimized)) {
        if (instance.asyncDep && !instance.asyncResolved) {
          updateComponentPreRender(instance, n2, optimized);
          return;
        } else {
          instance.next = n2;
          instance.update();
        }
      } else {
        n2.el = n1.el;
        instance.vnode = n2;
      }
    };
    const setupRenderEffect = (instance, initialVNode, container, anchor, parentSuspense, namespace, optimized) => {
      const componentUpdateFn = () => {
        if (!instance.isMounted) {
          let vnodeHook;
          const { el: el2, props } = initialVNode;
          const { bm, m, parent, root, type } = instance;
          const isAsyncWrapperVNode = isAsyncWrapper(initialVNode);
          toggleRecurse(instance, false);
          if (bm) {
            invokeArrayFns(bm);
          }
          if (!isAsyncWrapperVNode && (vnodeHook = props && props.onVnodeBeforeMount)) {
            invokeVNodeHook(vnodeHook, parent, initialVNode);
          }
          toggleRecurse(instance, true);
          {
            if (root.ce && root.ce._hasShadowRoot()) {
              root.ce._injectChildStyle(
                type,
                instance.parent ? instance.parent.type : void 0
              );
            }
            const subTree = instance.subTree = renderComponentRoot(instance);
            patch(
              null,
              subTree,
              container,
              anchor,
              instance,
              parentSuspense,
              namespace
            );
            initialVNode.el = subTree.el;
          }
          if (m) {
            queuePostRenderEffect(m, parentSuspense);
          }
          if (!isAsyncWrapperVNode && (vnodeHook = props && props.onVnodeMounted)) {
            const scopedInitialVNode = initialVNode;
            queuePostRenderEffect(
              () => invokeVNodeHook(vnodeHook, parent, scopedInitialVNode),
              parentSuspense
            );
          }
          if (initialVNode.shapeFlag & 256 || parent && isAsyncWrapper(parent.vnode) && parent.vnode.shapeFlag & 256) {
            instance.a && queuePostRenderEffect(instance.a, parentSuspense);
          }
          instance.isMounted = true;
          initialVNode = container = anchor = null;
        } else {
          let { next, bu, u, parent, vnode } = instance;
          {
            const nonHydratedAsyncRoot = locateNonHydratedAsyncRoot(instance);
            if (nonHydratedAsyncRoot) {
              if (next) {
                next.el = vnode.el;
                updateComponentPreRender(instance, next, optimized);
              }
              nonHydratedAsyncRoot.asyncDep.then(() => {
                queuePostRenderEffect(() => {
                  if (!instance.isUnmounted) update();
                }, parentSuspense);
              });
              return;
            }
          }
          let originNext = next;
          let vnodeHook;
          toggleRecurse(instance, false);
          if (next) {
            next.el = vnode.el;
            updateComponentPreRender(instance, next, optimized);
          } else {
            next = vnode;
          }
          if (bu) {
            invokeArrayFns(bu);
          }
          if (vnodeHook = next.props && next.props.onVnodeBeforeUpdate) {
            invokeVNodeHook(vnodeHook, parent, next, vnode);
          }
          toggleRecurse(instance, true);
          const nextTree = renderComponentRoot(instance);
          const prevTree = instance.subTree;
          instance.subTree = nextTree;
          patch(
            prevTree,
            nextTree,
            // parent may have changed if it's in a teleport
            hostParentNode(prevTree.el),
            // anchor may have changed if it's in a fragment
            getNextHostNode(prevTree),
            instance,
            parentSuspense,
            namespace
          );
          next.el = nextTree.el;
          if (originNext === null) {
            updateHOCHostEl(instance, nextTree.el);
          }
          if (u) {
            queuePostRenderEffect(u, parentSuspense);
          }
          if (vnodeHook = next.props && next.props.onVnodeUpdated) {
            queuePostRenderEffect(
              () => invokeVNodeHook(vnodeHook, parent, next, vnode),
              parentSuspense
            );
          }
        }
      };
      instance.scope.on();
      const effect2 = instance.effect = new ReactiveEffect(componentUpdateFn);
      instance.scope.off();
      const update = instance.update = effect2.run.bind(effect2);
      const job = instance.job = effect2.runIfDirty.bind(effect2);
      job.i = instance;
      job.id = instance.uid;
      effect2.scheduler = () => queueJob(job);
      toggleRecurse(instance, true);
      update();
    };
    const updateComponentPreRender = (instance, nextVNode, optimized) => {
      nextVNode.component = instance;
      const prevProps = instance.vnode.props;
      instance.vnode = nextVNode;
      instance.next = null;
      updateProps(instance, nextVNode.props, prevProps, optimized);
      updateSlots(instance, nextVNode.children, optimized);
      pauseTracking();
      flushPreFlushCbs(instance);
      resetTracking();
    };
    const patchChildren = (n1, n2, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized = false) => {
      const c1 = n1 && n1.children;
      const prevShapeFlag = n1 ? n1.shapeFlag : 0;
      const c2 = n2.children;
      const { patchFlag, shapeFlag } = n2;
      if (patchFlag > 0) {
        if (patchFlag & 128) {
          patchKeyedChildren(
            c1,
            c2,
            container,
            anchor,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
          return;
        } else if (patchFlag & 256) {
          patchUnkeyedChildren(
            c1,
            c2,
            container,
            anchor,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
          return;
        }
      }
      if (shapeFlag & 8) {
        if (prevShapeFlag & 16) {
          unmountChildren(c1, parentComponent, parentSuspense);
        }
        if (c2 !== c1) {
          hostSetElementText(container, c2);
        }
      } else {
        if (prevShapeFlag & 16) {
          if (shapeFlag & 16) {
            patchKeyedChildren(
              c1,
              c2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
          } else {
            unmountChildren(c1, parentComponent, parentSuspense, true);
          }
        } else {
          if (prevShapeFlag & 8) {
            hostSetElementText(container, "");
          }
          if (shapeFlag & 16) {
            mountChildren(
              c2,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
          }
        }
      }
    };
    const patchUnkeyedChildren = (c1, c2, container, anchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      c1 = c1 || EMPTY_ARR;
      c2 = c2 || EMPTY_ARR;
      const oldLength = c1.length;
      const newLength = c2.length;
      const commonLength = Math.min(oldLength, newLength);
      let i;
      for (i = 0; i < commonLength; i++) {
        const nextChild = c2[i] = optimized ? cloneIfMounted(c2[i]) : normalizeVNode(c2[i]);
        patch(
          c1[i],
          nextChild,
          container,
          null,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          optimized
        );
      }
      if (oldLength > newLength) {
        unmountChildren(
          c1,
          parentComponent,
          parentSuspense,
          true,
          false,
          commonLength
        );
      } else {
        mountChildren(
          c2,
          container,
          anchor,
          parentComponent,
          parentSuspense,
          namespace,
          slotScopeIds,
          optimized,
          commonLength
        );
      }
    };
    const patchKeyedChildren = (c1, c2, container, parentAnchor, parentComponent, parentSuspense, namespace, slotScopeIds, optimized) => {
      let i = 0;
      const l2 = c2.length;
      let e1 = c1.length - 1;
      let e2 = l2 - 1;
      while (i <= e1 && i <= e2) {
        const n1 = c1[i];
        const n2 = c2[i] = optimized ? cloneIfMounted(c2[i]) : normalizeVNode(c2[i]);
        if (isSameVNodeType(n1, n2)) {
          patch(
            n1,
            n2,
            container,
            null,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
        } else {
          break;
        }
        i++;
      }
      while (i <= e1 && i <= e2) {
        const n1 = c1[e1];
        const n2 = c2[e2] = optimized ? cloneIfMounted(c2[e2]) : normalizeVNode(c2[e2]);
        if (isSameVNodeType(n1, n2)) {
          patch(
            n1,
            n2,
            container,
            null,
            parentComponent,
            parentSuspense,
            namespace,
            slotScopeIds,
            optimized
          );
        } else {
          break;
        }
        e1--;
        e2--;
      }
      if (i > e1) {
        if (i <= e2) {
          const nextPos = e2 + 1;
          const anchor = nextPos < l2 ? c2[nextPos].el : parentAnchor;
          while (i <= e2) {
            patch(
              null,
              c2[i] = optimized ? cloneIfMounted(c2[i]) : normalizeVNode(c2[i]),
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
            i++;
          }
        }
      } else if (i > e2) {
        while (i <= e1) {
          unmount(c1[i], parentComponent, parentSuspense, true);
          i++;
        }
      } else {
        const s1 = i;
        const s2 = i;
        const keyToNewIndexMap = /* @__PURE__ */ new Map();
        for (i = s2; i <= e2; i++) {
          const nextChild = c2[i] = optimized ? cloneIfMounted(c2[i]) : normalizeVNode(c2[i]);
          if (nextChild.key != null) {
            keyToNewIndexMap.set(nextChild.key, i);
          }
        }
        let j;
        let patched = 0;
        const toBePatched = e2 - s2 + 1;
        let moved = false;
        let maxNewIndexSoFar = 0;
        const newIndexToOldIndexMap = new Array(toBePatched);
        for (i = 0; i < toBePatched; i++) newIndexToOldIndexMap[i] = 0;
        for (i = s1; i <= e1; i++) {
          const prevChild = c1[i];
          if (patched >= toBePatched) {
            unmount(prevChild, parentComponent, parentSuspense, true);
            continue;
          }
          let newIndex;
          if (prevChild.key != null) {
            newIndex = keyToNewIndexMap.get(prevChild.key);
          } else {
            for (j = s2; j <= e2; j++) {
              if (newIndexToOldIndexMap[j - s2] === 0 && isSameVNodeType(prevChild, c2[j])) {
                newIndex = j;
                break;
              }
            }
          }
          if (newIndex === void 0) {
            unmount(prevChild, parentComponent, parentSuspense, true);
          } else {
            newIndexToOldIndexMap[newIndex - s2] = i + 1;
            if (newIndex >= maxNewIndexSoFar) {
              maxNewIndexSoFar = newIndex;
            } else {
              moved = true;
            }
            patch(
              prevChild,
              c2[newIndex],
              container,
              null,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
            patched++;
          }
        }
        const increasingNewIndexSequence = moved ? getSequence(newIndexToOldIndexMap) : EMPTY_ARR;
        j = increasingNewIndexSequence.length - 1;
        for (i = toBePatched - 1; i >= 0; i--) {
          const nextIndex = s2 + i;
          const nextChild = c2[nextIndex];
          const anchorVNode = c2[nextIndex + 1];
          const anchor = nextIndex + 1 < l2 ? (
            // #13559, #14173 fallback to el placeholder for unresolved async component
            anchorVNode.el || resolveAsyncComponentPlaceholder(anchorVNode)
          ) : parentAnchor;
          if (newIndexToOldIndexMap[i] === 0) {
            patch(
              null,
              nextChild,
              container,
              anchor,
              parentComponent,
              parentSuspense,
              namespace,
              slotScopeIds,
              optimized
            );
          } else if (moved) {
            if (j < 0 || i !== increasingNewIndexSequence[j]) {
              move(nextChild, container, anchor, 2);
            } else {
              j--;
            }
          }
        }
      }
    };
    const move = (vnode, container, anchor, moveType, parentSuspense = null) => {
      const { el: el2, type, transition, children, shapeFlag } = vnode;
      if (shapeFlag & 6) {
        move(vnode.component.subTree, container, anchor, moveType);
        return;
      }
      if (shapeFlag & 128) {
        vnode.suspense.move(container, anchor, moveType);
        return;
      }
      if (shapeFlag & 64) {
        type.move(vnode, container, anchor, internals);
        return;
      }
      if (type === Fragment) {
        hostInsert(el2, container, anchor);
        for (let i = 0; i < children.length; i++) {
          move(children[i], container, anchor, moveType);
        }
        hostInsert(vnode.anchor, container, anchor);
        return;
      }
      if (type === Static) {
        moveStaticNode(vnode, container, anchor);
        return;
      }
      const needTransition2 = moveType !== 2 && shapeFlag & 1 && transition;
      if (needTransition2) {
        if (moveType === 0) {
          if (transition.persisted && !el2[leaveCbKey]) {
            hostInsert(el2, container, anchor);
          } else {
            transition.beforeEnter(el2);
            hostInsert(el2, container, anchor);
            queuePostRenderEffect(() => transition.enter(el2), parentSuspense);
          }
        } else {
          const { leave, delayLeave, afterLeave } = transition;
          const remove22 = () => {
            if (vnode.ctx.isUnmounted) {
              hostRemove(el2);
            } else {
              hostInsert(el2, container, anchor);
            }
          };
          const performLeave = () => {
            const wasLeaving = el2._isLeaving || !!el2[leaveCbKey];
            if (el2._isLeaving) {
              el2[leaveCbKey](
                true
                /* cancelled */
              );
            }
            if (transition.persisted && !wasLeaving) {
              remove22();
            } else {
              leave(el2, () => {
                remove22();
                afterLeave && afterLeave();
              });
            }
          };
          if (delayLeave) {
            delayLeave(el2, remove22, performLeave);
          } else {
            performLeave();
          }
        }
      } else {
        hostInsert(el2, container, anchor);
      }
    };
    const unmount = (vnode, parentComponent, parentSuspense, doRemove = false, optimized = false) => {
      const {
        type,
        props,
        ref: ref3,
        children,
        dynamicChildren,
        shapeFlag,
        patchFlag,
        dirs,
        cacheIndex,
        memo
      } = vnode;
      if (patchFlag === -2) {
        optimized = false;
      }
      if (ref3 != null) {
        pauseTracking();
        setRef(ref3, null, parentSuspense, vnode, true);
        resetTracking();
      }
      if (cacheIndex != null) {
        parentComponent.renderCache[cacheIndex] = void 0;
      }
      if (shapeFlag & 256) {
        parentComponent.ctx.deactivate(vnode);
        return;
      }
      const shouldInvokeDirs = shapeFlag & 1 && dirs;
      const shouldInvokeVnodeHook = !isAsyncWrapper(vnode);
      let vnodeHook;
      if (shouldInvokeVnodeHook && (vnodeHook = props && props.onVnodeBeforeUnmount)) {
        invokeVNodeHook(vnodeHook, parentComponent, vnode);
      }
      if (shapeFlag & 6) {
        unmountComponent(vnode.component, parentSuspense, doRemove);
      } else {
        if (shapeFlag & 128) {
          vnode.suspense.unmount(parentSuspense, doRemove);
          return;
        }
        if (shouldInvokeDirs) {
          invokeDirectiveHook(vnode, null, parentComponent, "beforeUnmount");
        }
        if (shapeFlag & 64) {
          vnode.type.remove(
            vnode,
            parentComponent,
            parentSuspense,
            internals,
            doRemove
          );
        } else if (dynamicChildren && // #5154
        // when v-once is used inside a block, setBlockTracking(-1) marks the
        // parent block with hasOnce: true
        // so that it doesn't take the fast path during unmount - otherwise
        // components nested in v-once are never unmounted.
        !dynamicChildren.hasOnce && // #1153: fast path should not be taken for non-stable (v-for) fragments
        (type !== Fragment || patchFlag > 0 && patchFlag & 64)) {
          unmountChildren(
            dynamicChildren,
            parentComponent,
            parentSuspense,
            false,
            true
          );
        } else if (type === Fragment && patchFlag & (128 | 256) || !optimized && shapeFlag & 16) {
          unmountChildren(children, parentComponent, parentSuspense);
        }
        if (doRemove) {
          remove2(vnode);
        }
      }
      const shouldInvalidateMemo = memo != null && cacheIndex == null;
      if (shouldInvokeVnodeHook && (vnodeHook = props && props.onVnodeUnmounted) || shouldInvokeDirs || shouldInvalidateMemo) {
        queuePostRenderEffect(() => {
          vnodeHook && invokeVNodeHook(vnodeHook, parentComponent, vnode);
          shouldInvokeDirs && invokeDirectiveHook(vnode, null, parentComponent, "unmounted");
          if (shouldInvalidateMemo) {
            vnode.el = null;
          }
        }, parentSuspense);
      }
    };
    const remove2 = (vnode) => {
      const { type, el: el2, anchor, transition } = vnode;
      if (type === Fragment) {
        {
          removeFragment(el2, anchor);
        }
        return;
      }
      if (type === Static) {
        removeStaticNode(vnode);
        return;
      }
      const performRemove = () => {
        hostRemove(el2);
        if (transition && !transition.persisted && transition.afterLeave) {
          transition.afterLeave();
        }
      };
      if (vnode.shapeFlag & 1 && transition && !transition.persisted) {
        const { leave, delayLeave } = transition;
        const performLeave = () => leave(el2, performRemove);
        if (delayLeave) {
          delayLeave(vnode.el, performRemove, performLeave);
        } else {
          performLeave();
        }
      } else {
        performRemove();
      }
    };
    const removeFragment = (cur, end) => {
      let next;
      while (cur !== end) {
        next = hostNextSibling(cur);
        hostRemove(cur);
        cur = next;
      }
      hostRemove(end);
    };
    const unmountComponent = (instance, parentSuspense, doRemove) => {
      const { bum, scope, job, subTree, um, m, a } = instance;
      invalidateMount(m);
      invalidateMount(a);
      if (bum) {
        invokeArrayFns(bum);
      }
      scope.stop();
      if (job) {
        job.flags |= 8;
        unmount(subTree, instance, parentSuspense, doRemove);
      }
      if (um) {
        queuePostRenderEffect(um, parentSuspense);
      }
      queuePostRenderEffect(() => {
        instance.isUnmounted = true;
      }, parentSuspense);
    };
    const unmountChildren = (children, parentComponent, parentSuspense, doRemove = false, optimized = false, start = 0) => {
      for (let i = start; i < children.length; i++) {
        unmount(children[i], parentComponent, parentSuspense, doRemove, optimized);
      }
    };
    const getNextHostNode = (vnode) => {
      if (vnode.shapeFlag & 6) {
        return getNextHostNode(vnode.component.subTree);
      }
      if (vnode.shapeFlag & 128) {
        return vnode.suspense.next();
      }
      const el2 = hostNextSibling(vnode.anchor || vnode.el);
      const teleportEnd = el2 && el2[TeleportEndKey];
      return teleportEnd ? hostNextSibling(teleportEnd) : el2;
    };
    let isFlushing = false;
    const render = (vnode, container, namespace) => {
      let instance;
      if (vnode == null) {
        if (container._vnode) {
          unmount(container._vnode, null, null, true);
          instance = container._vnode.component;
        }
      } else {
        patch(
          container._vnode || null,
          vnode,
          container,
          null,
          null,
          null,
          namespace
        );
      }
      container._vnode = vnode;
      if (!isFlushing) {
        isFlushing = true;
        flushPreFlushCbs(instance);
        flushPostFlushCbs();
        isFlushing = false;
      }
    };
    const internals = {
      p: patch,
      um: unmount,
      m: move,
      r: remove2,
      mt: mountComponent,
      mc: mountChildren,
      pc: patchChildren,
      pbc: patchBlockChildren,
      n: getNextHostNode,
      o: options
    };
    let hydrate;
    return {
      render,
      hydrate,
      createApp: createAppAPI(render)
    };
  }
  function resolveChildrenNamespace({ type, props }, currentNamespace) {
    return currentNamespace === "svg" && type === "foreignObject" || currentNamespace === "mathml" && type === "annotation-xml" && props && props.encoding && props.encoding.includes("html") ? void 0 : currentNamespace;
  }
  function toggleRecurse({ effect: effect2, job }, allowed) {
    if (allowed) {
      effect2.flags |= 32;
      job.flags |= 4;
    } else {
      effect2.flags &= -33;
      job.flags &= -5;
    }
  }
  function needTransition(parentSuspense, transition) {
    return (!parentSuspense || parentSuspense && !parentSuspense.pendingBranch) && transition && !transition.persisted;
  }
  function traverseStaticChildren(n1, n2, shallow = false) {
    const ch1 = n1.children;
    const ch2 = n2.children;
    if (isArray(ch1) && isArray(ch2)) {
      for (let i = 0; i < ch1.length; i++) {
        const c1 = ch1[i];
        let c2 = ch2[i];
        if (c2.shapeFlag & 1 && !c2.dynamicChildren) {
          if (c2.patchFlag <= 0 || c2.patchFlag === 32) {
            c2 = ch2[i] = cloneIfMounted(ch2[i]);
            c2.el = c1.el;
          }
          if (!shallow && c2.patchFlag !== -2)
            traverseStaticChildren(c1, c2);
        }
        if (c2.type === Text) {
          if (c2.patchFlag === -1) {
            c2 = ch2[i] = cloneIfMounted(c2);
          }
          c2.el = c1.el;
        }
        if (c2.type === Comment && !c2.el) {
          c2.el = c1.el;
        }
      }
    }
  }
  function getSequence(arr) {
    const p2 = arr.slice();
    const result = [0];
    let i, j, u, v, c;
    const len = arr.length;
    for (i = 0; i < len; i++) {
      const arrI = arr[i];
      if (arrI !== 0) {
        j = result[result.length - 1];
        if (arr[j] < arrI) {
          p2[i] = j;
          result.push(i);
          continue;
        }
        u = 0;
        v = result.length - 1;
        while (u < v) {
          c = u + v >> 1;
          if (arr[result[c]] < arrI) {
            u = c + 1;
          } else {
            v = c;
          }
        }
        if (arrI < arr[result[u]]) {
          if (u > 0) {
            p2[i] = result[u - 1];
          }
          result[u] = i;
        }
      }
    }
    u = result.length;
    v = result[u - 1];
    while (u-- > 0) {
      result[u] = v;
      v = p2[v];
    }
    return result;
  }
  function locateNonHydratedAsyncRoot(instance) {
    const subComponent = instance.subTree.component;
    if (subComponent) {
      if (subComponent.asyncDep && !subComponent.asyncResolved) {
        return subComponent;
      } else {
        return locateNonHydratedAsyncRoot(subComponent);
      }
    }
  }
  function invalidateMount(hooks) {
    if (hooks) {
      for (let i = 0; i < hooks.length; i++)
        hooks[i].flags |= 8;
    }
  }
  function resolveAsyncComponentPlaceholder(anchorVnode) {
    if (anchorVnode.placeholder) {
      return anchorVnode.placeholder;
    }
    const instance = anchorVnode.component;
    if (instance) {
      return resolveAsyncComponentPlaceholder(instance.subTree);
    }
    return null;
  }
  const isSuspense = (type) => type.__isSuspense;
  function queueEffectWithSuspense(fn, suspense) {
    if (suspense && suspense.pendingBranch) {
      if (isArray(fn)) {
        suspense.effects.push(...fn);
      } else {
        suspense.effects.push(fn);
      }
    } else {
      queuePostFlushCb(fn);
    }
  }
  const Fragment = /* @__PURE__ */ Symbol.for("v-fgt");
  const Text = /* @__PURE__ */ Symbol.for("v-txt");
  const Comment = /* @__PURE__ */ Symbol.for("v-cmt");
  const Static = /* @__PURE__ */ Symbol.for("v-stc");
  const blockStack = [];
  let currentBlock = null;
  function openBlock(disableTracking = false) {
    blockStack.push(currentBlock = disableTracking ? null : []);
  }
  function closeBlock() {
    blockStack.pop();
    currentBlock = blockStack[blockStack.length - 1] || null;
  }
  let isBlockTreeEnabled = 1;
  function setBlockTracking(value, inVOnce = false) {
    isBlockTreeEnabled += value;
    if (value < 0 && currentBlock && inVOnce) {
      currentBlock.hasOnce = true;
    }
  }
  function setupBlock(vnode) {
    vnode.dynamicChildren = isBlockTreeEnabled > 0 ? currentBlock || EMPTY_ARR : null;
    closeBlock();
    if (isBlockTreeEnabled > 0 && currentBlock) {
      currentBlock.push(vnode);
    }
    return vnode;
  }
  function createElementBlock(type, props, children, patchFlag, dynamicProps, shapeFlag) {
    return setupBlock(
      createBaseVNode(
        type,
        props,
        children,
        patchFlag,
        dynamicProps,
        shapeFlag,
        true
      )
    );
  }
  function createBlock(type, props, children, patchFlag, dynamicProps) {
    return setupBlock(
      createVNode(
        type,
        props,
        children,
        patchFlag,
        dynamicProps,
        true
      )
    );
  }
  function isVNode(value) {
    return value ? value.__v_isVNode === true : false;
  }
  function isSameVNodeType(n1, n2) {
    return n1.type === n2.type && n1.key === n2.key;
  }
  const normalizeKey = ({ key }) => key != null ? key : null;
  const normalizeRef = ({
    ref: ref3,
    ref_key,
    ref_for
  }) => {
    if (typeof ref3 === "number") {
      ref3 = "" + ref3;
    }
    return ref3 != null ? isString(ref3) || /* @__PURE__ */ isRef(ref3) || isFunction(ref3) ? { i: currentRenderingInstance, r: ref3, k: ref_key, f: !!ref_for } : ref3 : null;
  };
  function createBaseVNode(type, props = null, children = null, patchFlag = 0, dynamicProps = null, shapeFlag = type === Fragment ? 0 : 1, isBlockNode = false, needFullChildrenNormalization = false) {
    const vnode = {
      __v_isVNode: true,
      __v_skip: true,
      type,
      props,
      key: props && normalizeKey(props),
      ref: props && normalizeRef(props),
      scopeId: currentScopeId,
      slotScopeIds: null,
      children,
      component: null,
      suspense: null,
      ssContent: null,
      ssFallback: null,
      dirs: null,
      transition: null,
      el: null,
      anchor: null,
      target: null,
      targetStart: null,
      targetAnchor: null,
      staticCount: 0,
      shapeFlag,
      patchFlag,
      dynamicProps,
      dynamicChildren: null,
      appContext: null,
      ctx: currentRenderingInstance
    };
    if (needFullChildrenNormalization) {
      normalizeChildren(vnode, children);
      if (shapeFlag & 128) {
        type.normalize(vnode);
      }
    } else if (children) {
      vnode.shapeFlag |= isString(children) ? 8 : 16;
    }
    if (isBlockTreeEnabled > 0 && // avoid a block node from tracking itself
    !isBlockNode && // has current parent block
    currentBlock && // presence of a patch flag indicates this node needs patching on updates.
    // component nodes also should always be patched, because even if the
    // component doesn't need to update, it needs to persist the instance on to
    // the next vnode so that it can be properly unmounted later.
    (vnode.patchFlag > 0 || shapeFlag & 6) && // the EVENTS flag is only for hydration and if it is the only flag, the
    // vnode should not be considered dynamic due to handler caching.
    vnode.patchFlag !== 32) {
      currentBlock.push(vnode);
    }
    return vnode;
  }
  const createVNode = _createVNode;
  function _createVNode(type, props = null, children = null, patchFlag = 0, dynamicProps = null, isBlockNode = false) {
    if (!type || type === NULL_DYNAMIC_COMPONENT) {
      type = Comment;
    }
    if (isVNode(type)) {
      const cloned = cloneVNode(
        type,
        props,
        true
        /* mergeRef: true */
      );
      if (children) {
        normalizeChildren(cloned, children);
      }
      if (isBlockTreeEnabled > 0 && !isBlockNode && currentBlock) {
        if (cloned.shapeFlag & 6) {
          currentBlock[currentBlock.indexOf(type)] = cloned;
        } else {
          currentBlock.push(cloned);
        }
      }
      cloned.patchFlag = -2;
      return cloned;
    }
    if (isClassComponent(type)) {
      type = type.__vccOpts;
    }
    if (props) {
      props = guardReactiveProps(props);
      let { class: klass, style } = props;
      if (klass && !isString(klass)) {
        props.class = normalizeClass(klass);
      }
      if (isObject(style)) {
        if (/* @__PURE__ */ isProxy(style) && !isArray(style)) {
          style = extend({}, style);
        }
        props.style = normalizeStyle(style);
      }
    }
    const shapeFlag = isString(type) ? 1 : isSuspense(type) ? 128 : isTeleport(type) ? 64 : isObject(type) ? 4 : isFunction(type) ? 2 : 0;
    return createBaseVNode(
      type,
      props,
      children,
      patchFlag,
      dynamicProps,
      shapeFlag,
      isBlockNode,
      true
    );
  }
  function guardReactiveProps(props) {
    if (!props) return null;
    return /* @__PURE__ */ isProxy(props) || isInternalObject(props) ? extend({}, props) : props;
  }
  function cloneVNode(vnode, extraProps, mergeRef = false, cloneTransition = false) {
    const { props, ref: ref3, patchFlag, children, transition } = vnode;
    const mergedProps = extraProps ? mergeProps(props || {}, extraProps) : props;
    const cloned = {
      __v_isVNode: true,
      __v_skip: true,
      type: vnode.type,
      props: mergedProps,
      key: mergedProps && normalizeKey(mergedProps),
      ref: extraProps && extraProps.ref ? (
        // #2078 in the case of <component :is="vnode" ref="extra"/>
        // if the vnode itself already has a ref, cloneVNode will need to merge
        // the refs so the single vnode can be set on multiple refs
        mergeRef && ref3 ? isArray(ref3) ? ref3.concat(normalizeRef(extraProps)) : [ref3, normalizeRef(extraProps)] : normalizeRef(extraProps)
      ) : ref3,
      scopeId: vnode.scopeId,
      slotScopeIds: vnode.slotScopeIds,
      children,
      target: vnode.target,
      targetStart: vnode.targetStart,
      targetAnchor: vnode.targetAnchor,
      staticCount: vnode.staticCount,
      shapeFlag: vnode.shapeFlag,
      // if the vnode is cloned with extra props, we can no longer assume its
      // existing patch flag to be reliable and need to add the FULL_PROPS flag.
      // note: preserve flag for fragments since they use the flag for children
      // fast paths only.
      patchFlag: extraProps && vnode.type !== Fragment ? patchFlag === -1 ? 16 : patchFlag | 16 : patchFlag,
      dynamicProps: vnode.dynamicProps,
      dynamicChildren: vnode.dynamicChildren,
      appContext: vnode.appContext,
      dirs: vnode.dirs,
      transition,
      // These should technically only be non-null on mounted VNodes. However,
      // they *should* be copied for kept-alive vnodes. So we just always copy
      // them since them being non-null during a mount doesn't affect the logic as
      // they will simply be overwritten.
      component: vnode.component,
      suspense: vnode.suspense,
      ssContent: vnode.ssContent && cloneVNode(vnode.ssContent),
      ssFallback: vnode.ssFallback && cloneVNode(vnode.ssFallback),
      placeholder: vnode.placeholder,
      el: vnode.el,
      anchor: vnode.anchor,
      ctx: vnode.ctx,
      ce: vnode.ce
    };
    if (transition && cloneTransition) {
      setTransitionHooks(
        cloned,
        transition.clone(cloned)
      );
    }
    return cloned;
  }
  function createTextVNode(text = " ", flag = 0) {
    return createVNode(Text, null, text, flag);
  }
  function createStaticVNode(content, numberOfNodes) {
    const vnode = createVNode(Static, null, content);
    vnode.staticCount = numberOfNodes;
    return vnode;
  }
  function createCommentVNode(text = "", asBlock = false) {
    return asBlock ? (openBlock(), createBlock(Comment, null, text)) : createVNode(Comment, null, text);
  }
  function normalizeVNode(child) {
    if (child == null || typeof child === "boolean") {
      return createVNode(Comment);
    } else if (isArray(child)) {
      return createVNode(
        Fragment,
        null,
        // #3666, avoid reference pollution when reusing vnode
        child.slice()
      );
    } else if (isVNode(child)) {
      return cloneIfMounted(child);
    } else {
      return createVNode(Text, null, String(child));
    }
  }
  function cloneIfMounted(child) {
    return child.el === null && child.patchFlag !== -1 || child.memo ? child : cloneVNode(child);
  }
  function normalizeChildren(vnode, children) {
    let type = 0;
    const { shapeFlag } = vnode;
    if (children == null) {
      children = null;
    } else if (isArray(children)) {
      type = 16;
    } else if (typeof children === "object") {
      if (shapeFlag & (1 | 64)) {
        const slot = children.default;
        if (slot) {
          slot._c && (slot._d = false);
          normalizeChildren(vnode, slot());
          slot._c && (slot._d = true);
        }
        return;
      } else {
        type = 32;
        const slotFlag = children._;
        if (!slotFlag && !isInternalObject(children)) {
          children._ctx = currentRenderingInstance;
        } else if (slotFlag === 3 && currentRenderingInstance) {
          if (currentRenderingInstance.slots._ === 1) {
            children._ = 1;
          } else {
            children._ = 2;
            vnode.patchFlag |= 1024;
          }
        }
      }
    } else if (isFunction(children)) {
      if (shapeFlag & (1 | 64)) {
        normalizeChildren(vnode, { default: children });
        return;
      }
      children = { default: children, _ctx: currentRenderingInstance };
      type = 32;
    } else {
      children = String(children);
      if (shapeFlag & 64) {
        type = 16;
        children = [createTextVNode(children)];
      } else {
        type = 8;
      }
    }
    vnode.children = children;
    vnode.shapeFlag |= type;
  }
  function mergeProps(...args) {
    const ret = {};
    for (let i = 0; i < args.length; i++) {
      const toMerge = args[i];
      for (const key in toMerge) {
        if (key === "class") {
          if (ret.class !== toMerge.class) {
            ret.class = normalizeClass([ret.class, toMerge.class]);
          }
        } else if (key === "style") {
          ret.style = normalizeStyle([ret.style, toMerge.style]);
        } else if (isOn(key)) {
          const existing = ret[key];
          const incoming = toMerge[key];
          if (incoming && existing !== incoming && !(isArray(existing) && existing.includes(incoming))) {
            ret[key] = existing ? [].concat(existing, incoming) : incoming;
          } else if (incoming == null && existing == null && // mergeProps({ 'onUpdate:modelValue': undefined }) should not retain
          // the model listener.
          !isModelListener(key)) {
            ret[key] = incoming;
          }
        } else if (key !== "") {
          ret[key] = toMerge[key];
        }
      }
    }
    return ret;
  }
  function invokeVNodeHook(hook, instance, vnode, prevVNode = null) {
    callWithAsyncErrorHandling(hook, instance, 7, [
      vnode,
      prevVNode
    ]);
  }
  const emptyAppContext = createAppContext();
  let uid = 0;
  function createComponentInstance(vnode, parent, suspense) {
    const type = vnode.type;
    const appContext = (parent ? parent.appContext : vnode.appContext) || emptyAppContext;
    const instance = {
      uid: uid++,
      vnode,
      type,
      parent,
      appContext,
      root: null,
      // to be immediately set
      next: null,
      subTree: null,
      // will be set synchronously right after creation
      effect: null,
      update: null,
      // will be set synchronously right after creation
      job: null,
      scope: new EffectScope(
        true
        /* detached */
      ),
      render: null,
      proxy: null,
      exposed: null,
      exposeProxy: null,
      withProxy: null,
      provides: parent ? parent.provides : Object.create(appContext.provides),
      ids: parent ? parent.ids : ["", 0, 0],
      accessCache: null,
      renderCache: [],
      // local resolved assets
      components: null,
      directives: null,
      // resolved props and emits options
      propsOptions: normalizePropsOptions(type, appContext),
      emitsOptions: normalizeEmitsOptions(type, appContext),
      // emit
      emit: null,
      // to be set immediately
      emitted: null,
      // props default value
      propsDefaults: EMPTY_OBJ,
      // inheritAttrs
      inheritAttrs: type.inheritAttrs,
      // state
      ctx: EMPTY_OBJ,
      data: EMPTY_OBJ,
      props: EMPTY_OBJ,
      attrs: EMPTY_OBJ,
      slots: EMPTY_OBJ,
      refs: EMPTY_OBJ,
      setupState: EMPTY_OBJ,
      setupContext: null,
      // suspense related
      suspense,
      suspenseId: suspense ? suspense.pendingId : 0,
      asyncDep: null,
      asyncResolved: false,
      // lifecycle hooks
      // not using enums here because it results in computed properties
      isMounted: false,
      isUnmounted: false,
      isDeactivated: false,
      bc: null,
      c: null,
      bm: null,
      m: null,
      bu: null,
      u: null,
      um: null,
      bum: null,
      da: null,
      a: null,
      rtg: null,
      rtc: null,
      ec: null,
      sp: null
    };
    {
      instance.ctx = { _: instance };
    }
    instance.root = parent ? parent.root : instance;
    instance.emit = emit.bind(null, instance);
    if (vnode.ce) {
      vnode.ce(instance);
    }
    return instance;
  }
  let currentInstance = null;
  const getCurrentInstance = () => currentInstance || currentRenderingInstance;
  let internalSetCurrentInstance;
  let setInSSRSetupState;
  {
    const g = getGlobalThis();
    const registerGlobalSetter = (key, setter) => {
      let setters;
      if (!(setters = g[key])) setters = g[key] = [];
      setters.push(setter);
      return (v) => {
        if (setters.length > 1) setters.forEach((set) => set(v));
        else setters[0](v);
      };
    };
    internalSetCurrentInstance = registerGlobalSetter(
      `__VUE_INSTANCE_SETTERS__`,
      (v) => currentInstance = v
    );
    setInSSRSetupState = registerGlobalSetter(
      `__VUE_SSR_SETTERS__`,
      (v) => isInSSRComponentSetup = v
    );
  }
  const setCurrentInstance = (instance) => {
    const prev = currentInstance;
    internalSetCurrentInstance(instance);
    instance.scope.on();
    return () => {
      instance.scope.off();
      internalSetCurrentInstance(prev);
    };
  };
  const unsetCurrentInstance = () => {
    currentInstance && currentInstance.scope.off();
    internalSetCurrentInstance(null);
  };
  function isStatefulComponent(instance) {
    return instance.vnode.shapeFlag & 4;
  }
  let isInSSRComponentSetup = false;
  function setupComponent(instance, isSSR = false, optimized = false) {
    isSSR && setInSSRSetupState(isSSR);
    const { props, children } = instance.vnode;
    const isStateful = isStatefulComponent(instance);
    initProps(instance, props, isStateful, isSSR);
    initSlots(instance, children, optimized || isSSR);
    const setupResult = isStateful ? setupStatefulComponent(instance, isSSR) : void 0;
    isSSR && setInSSRSetupState(false);
    return setupResult;
  }
  function setupStatefulComponent(instance, isSSR) {
    const Component = instance.type;
    instance.accessCache = /* @__PURE__ */ Object.create(null);
    instance.proxy = new Proxy(instance.ctx, PublicInstanceProxyHandlers);
    const { setup } = Component;
    if (setup) {
      pauseTracking();
      const setupContext = instance.setupContext = setup.length > 1 ? createSetupContext(instance) : null;
      const reset = setCurrentInstance(instance);
      const setupResult = callWithErrorHandling(
        setup,
        instance,
        0,
        [
          instance.props,
          setupContext
        ]
      );
      const isAsyncSetup = isPromise(setupResult);
      resetTracking();
      reset();
      if ((isAsyncSetup || instance.sp) && !isAsyncWrapper(instance)) {
        markAsyncBoundary(instance);
      }
      if (isAsyncSetup) {
        setupResult.then(unsetCurrentInstance, unsetCurrentInstance);
        if (isSSR) {
          return setupResult.then((resolvedResult) => {
            setInSSRSetupState(true);
            try {
              handleSetupResult(instance, resolvedResult, isSSR);
            } finally {
              setInSSRSetupState(false);
            }
          }).catch((e) => {
            handleError(e, instance, 0);
          });
        } else {
          instance.asyncDep = setupResult;
        }
      } else {
        handleSetupResult(instance, setupResult);
      }
    } else {
      finishComponentSetup(instance);
    }
  }
  function handleSetupResult(instance, setupResult, isSSR) {
    if (isFunction(setupResult)) {
      if (instance.type.__ssrInlineRender) {
        instance.ssrRender = setupResult;
      } else {
        instance.render = setupResult;
      }
    } else if (isObject(setupResult)) {
      instance.setupState = proxyRefs(setupResult);
    } else ;
    finishComponentSetup(instance);
  }
  function finishComponentSetup(instance, isSSR, skipOptions) {
    const Component = instance.type;
    if (!instance.render) {
      instance.render = Component.render || NOOP;
    }
    {
      const reset = setCurrentInstance(instance);
      pauseTracking();
      try {
        applyOptions(instance);
      } finally {
        resetTracking();
        reset();
      }
    }
  }
  const attrsProxyHandlers = {
    get(target, key) {
      track(target, "get", "");
      return target[key];
    }
  };
  function createSetupContext(instance) {
    const expose = (exposed) => {
      instance.exposed = exposed || {};
    };
    {
      return {
        attrs: new Proxy(instance.attrs, attrsProxyHandlers),
        slots: instance.slots,
        emit: instance.emit,
        expose
      };
    }
  }
  function getComponentPublicInstance(instance) {
    if (instance.exposed) {
      return instance.exposeProxy || (instance.exposeProxy = new Proxy(proxyRefs(markRaw(instance.exposed)), {
        get(target, key) {
          if (key in target) {
            return target[key];
          } else if (key in publicPropertiesMap) {
            return publicPropertiesMap[key](instance);
          }
        },
        has(target, key) {
          return key in target || key in publicPropertiesMap;
        }
      }));
    } else {
      return instance.proxy;
    }
  }
  const classifyRE = /(?:^|[-_])\w/g;
  const classify = (str) => str.replace(classifyRE, (c) => c.toUpperCase()).replace(/[-_]/g, "");
  function getComponentName(Component, includeInferred = true) {
    return isFunction(Component) ? Component.displayName || Component.name : Component.name || includeInferred && Component.__name;
  }
  function formatComponentName(instance, Component, isRoot = false) {
    let name = getComponentName(Component);
    if (!name && Component.__file) {
      const match = Component.__file.match(/([^/\\]+)\.\w+$/);
      if (match) {
        name = match[1];
      }
    }
    if (!name && instance) {
      const inferFromRegistry = (registry) => {
        for (const key in registry) {
          if (registry[key] === Component) {
            return key;
          }
        }
      };
      name = inferFromRegistry(instance.components) || instance.parent && inferFromRegistry(
        instance.parent.type.components
      ) || inferFromRegistry(instance.appContext.components);
    }
    return name ? classify(name) : isRoot ? `App` : `Anonymous`;
  }
  function isClassComponent(value) {
    return isFunction(value) && "__vccOpts" in value;
  }
  const computed = (getterOrOptions, debugOptions) => {
    const c = /* @__PURE__ */ computed$1(getterOrOptions, debugOptions, isInSSRComponentSetup);
    return c;
  };
  const version = "3.5.42";
  /**
  * @vue/runtime-dom v3.5.42
  * (c) 2018-present Yuxi (Evan) You and Vue contributors
  * @license MIT
  **/
  let policy = void 0;
  const tt = typeof window !== "undefined" && window.trustedTypes;
  if (tt) {
    try {
      policy = /* @__PURE__ */ tt.createPolicy("vue", {
        createHTML: (val) => val
      });
    } catch (e) {
    }
  }
  const unsafeToTrustedHTML = policy ? (val) => policy.createHTML(val) : (val) => val;
  const svgNS = "http://www.w3.org/2000/svg";
  const mathmlNS = "http://www.w3.org/1998/Math/MathML";
  const doc = typeof document !== "undefined" ? document : null;
  const templateContainer = doc && /* @__PURE__ */ doc.createElement("template");
  const nodeOps = {
    insert: (child, parent, anchor) => {
      parent.insertBefore(child, anchor || null);
    },
    remove: (child) => {
      const parent = child.parentNode;
      if (parent) {
        parent.removeChild(child);
      }
    },
    createElement: (tag, namespace, is, props) => {
      const el2 = namespace === "svg" ? doc.createElementNS(svgNS, tag) : namespace === "mathml" ? doc.createElementNS(mathmlNS, tag) : is ? doc.createElement(tag, { is }) : doc.createElement(tag);
      if (tag === "select" && props && props.multiple != null) {
        el2.setAttribute("multiple", props.multiple);
      }
      return el2;
    },
    createText: (text) => doc.createTextNode(text),
    createComment: (text) => doc.createComment(text),
    setText: (node, text) => {
      node.nodeValue = text;
    },
    setElementText: (el2, text) => {
      el2.textContent = text;
    },
    parentNode: (node) => node.parentNode,
    nextSibling: (node) => node.nextSibling,
    querySelector: (selector) => doc.querySelector(selector),
    setScopeId(el2, id) {
      el2.setAttribute(id, "");
    },
    // __UNSAFE__
    // Reason: innerHTML.
    // Static content here can only come from compiled templates.
    // As long as the user only uses trusted templates, this is safe.
    insertStaticContent(content, parent, anchor, namespace, start, end) {
      const before = anchor ? anchor.previousSibling : parent.lastChild;
      if (start && (start === end || start.nextSibling)) {
        while (true) {
          parent.insertBefore(start.cloneNode(true), anchor);
          if (start === end || !(start = start.nextSibling)) break;
        }
      } else {
        templateContainer.innerHTML = unsafeToTrustedHTML(
          namespace === "svg" ? `<svg>${content}</svg>` : namespace === "mathml" ? `<math>${content}</math>` : content
        );
        const template = templateContainer.content;
        if (namespace === "svg" || namespace === "mathml") {
          const wrapper = template.firstChild;
          while (wrapper.firstChild) {
            template.appendChild(wrapper.firstChild);
          }
          template.removeChild(wrapper);
        }
        parent.insertBefore(template, anchor);
      }
      return [
        // first
        before ? before.nextSibling : parent.firstChild,
        // last
        anchor ? anchor.previousSibling : parent.lastChild
      ];
    }
  };
  const vtcKey = /* @__PURE__ */ Symbol("_vtc");
  function patchClass(el2, value, isSVG) {
    const transitionClasses = el2[vtcKey];
    if (transitionClasses) {
      value = (value ? [value, ...transitionClasses] : [...transitionClasses]).join(" ");
    }
    if (value == null) {
      el2.removeAttribute("class");
    } else if (isSVG) {
      el2.setAttribute("class", value);
    } else {
      el2.className = value;
    }
  }
  const vShowOriginalDisplay = /* @__PURE__ */ Symbol("_vod");
  const vShowHidden = /* @__PURE__ */ Symbol("_vsh");
  const CSS_VAR_TEXT = /* @__PURE__ */ Symbol("");
  const displayRE = /(?:^|;)\s*display\s*:/;
  function patchStyle(el2, prev, next) {
    const style = el2.style;
    const isCssString = isString(next);
    let hasControlledDisplay = false;
    if (next && !isCssString) {
      if (prev) {
        if (!isString(prev)) {
          for (const key in prev) {
            if (next[key] == null) {
              setStyle(style, key, "");
            }
          }
        } else {
          for (const prevStyle of prev.split(";")) {
            const key = prevStyle.slice(0, prevStyle.indexOf(":")).trim();
            if (next[key] == null) {
              setStyle(style, key, "");
            }
          }
        }
      }
      for (const key in next) {
        if (key === "display") {
          hasControlledDisplay = true;
        }
        const value = next[key];
        if (value != null) {
          if (!shouldPreserveTextareaResizeStyle(
            el2,
            key,
            !isString(prev) && prev ? prev[key] : void 0,
            value
          )) {
            setStyle(style, key, value);
          }
        } else {
          setStyle(style, key, "");
        }
      }
    } else {
      if (isCssString) {
        if (prev !== next) {
          const cssVarText = style[CSS_VAR_TEXT];
          if (cssVarText) {
            next += ";" + cssVarText;
          }
          style.cssText = next;
          hasControlledDisplay = displayRE.test(next);
        }
      } else if (prev) {
        el2.removeAttribute("style");
      }
    }
    if (vShowOriginalDisplay in el2) {
      el2[vShowOriginalDisplay] = hasControlledDisplay ? style.display : "";
      if (el2[vShowHidden]) {
        style.display = "none";
      }
    }
  }
  const importantRE = /\s*!important$/;
  function setStyle(style, name, val) {
    if (isArray(val)) {
      val.forEach((v) => setStyle(style, name, v));
    } else {
      if (val == null) val = "";
      if (name.startsWith("--")) {
        if (importantRE.test(val)) {
          style.setProperty(name, val.replace(importantRE, ""), "important");
        } else {
          style.setProperty(name, val);
        }
      } else {
        const prefixed = autoPrefix(style, name);
        if (importantRE.test(val)) {
          style.setProperty(
            hyphenate(prefixed),
            val.replace(importantRE, ""),
            "important"
          );
        } else {
          style[prefixed] = val;
        }
      }
    }
  }
  const prefixes = ["Webkit", "Moz", "ms"];
  const prefixCache = {};
  function autoPrefix(style, rawName) {
    const cached = prefixCache[rawName];
    if (cached) {
      return cached;
    }
    let name = camelize(rawName);
    if (name !== "filter" && name in style) {
      return prefixCache[rawName] = name;
    }
    name = capitalize(name);
    for (let i = 0; i < prefixes.length; i++) {
      const prefixed = prefixes[i] + name;
      if (prefixed in style) {
        return prefixCache[rawName] = prefixed;
      }
    }
    return rawName;
  }
  function shouldPreserveTextareaResizeStyle(el2, key, prev, next) {
    return el2.tagName === "TEXTAREA" && (key === "width" || key === "height") && isString(next) && prev === next;
  }
  const xlinkNS = "http://www.w3.org/1999/xlink";
  function patchAttr(el2, key, value, isSVG, instance, isBoolean = isSpecialBooleanAttr(key)) {
    if (isSVG && key.startsWith("xlink:")) {
      if (value == null) {
        el2.removeAttributeNS(xlinkNS, key.slice(6, key.length));
      } else {
        el2.setAttributeNS(xlinkNS, key, value);
      }
    } else {
      if (value == null || isBoolean && !includeBooleanAttr(value)) {
        el2.removeAttribute(key);
      } else {
        el2.setAttribute(
          key,
          isBoolean ? "" : isSymbol(value) ? String(value) : value
        );
      }
    }
  }
  function patchDOMProp(el2, key, value, parentComponent, attrName) {
    if (key === "innerHTML" || key === "textContent") {
      if (value != null) {
        el2[key] = key === "innerHTML" ? unsafeToTrustedHTML(value) : value;
      }
      return;
    }
    const tag = el2.tagName;
    if (key === "value" && tag !== "PROGRESS" && // custom elements may use _value internally
    !tag.includes("-")) {
      const oldValue = tag === "OPTION" ? el2.getAttribute("value") || "" : el2.value;
      const newValue = value == null ? (
        // #11647: value should be set as empty string for null and undefined,
        // but <input type="checkbox"> should be set as 'on'.
        el2.type === "checkbox" ? "on" : ""
      ) : String(value);
      if (oldValue !== newValue || !("_value" in el2)) {
        el2.value = newValue;
      }
      if (value == null) {
        el2.removeAttribute(key);
      }
      el2._value = value;
      return;
    }
    let needRemove = false;
    if (value === "" || value == null) {
      const type = typeof el2[key];
      if (type === "boolean") {
        value = includeBooleanAttr(value);
      } else if (value == null && type === "string") {
        value = "";
        needRemove = true;
      } else if (type === "number") {
        value = 0;
        needRemove = true;
      }
    }
    try {
      el2[key] = value;
    } catch (e) {
    }
    needRemove && el2.removeAttribute(attrName || key);
  }
  function addEventListener(el2, event, handler, options) {
    el2.addEventListener(event, handler, options);
  }
  function removeEventListener(el2, event, handler, options) {
    el2.removeEventListener(event, handler, options);
  }
  const veiKey = /* @__PURE__ */ Symbol("_vei");
  function patchEvent(el2, rawName, prevValue, nextValue, instance = null) {
    const invokers = el2[veiKey] || (el2[veiKey] = {});
    const existingInvoker = invokers[rawName];
    if (nextValue && existingInvoker) {
      existingInvoker.value = nextValue;
    } else {
      const [name, options] = parseName(rawName);
      if (nextValue) {
        const invoker = invokers[rawName] = createInvoker(
          nextValue,
          instance
        );
        addEventListener(el2, name, invoker, options);
      } else if (existingInvoker) {
        removeEventListener(el2, name, existingInvoker, options);
        invokers[rawName] = void 0;
      }
    }
  }
  const optionsModifierRE = /(Once|Passive|Capture)$/;
  const optionsModifierEventRE = /^on:?(?:Once|Passive|Capture)$/;
  function parseName(name) {
    let options;
    let m;
    while ((m = name.match(optionsModifierRE)) && !optionsModifierEventRE.test(name)) {
      if (!options) options = {};
      name = name.slice(0, name.length - m[1].length);
      options[m[1].toLowerCase()] = true;
    }
    const event = name[2] === ":" ? name.slice(3) : hyphenate(name.slice(2));
    return [event, options];
  }
  let cachedNow = 0;
  const p = /* @__PURE__ */ Promise.resolve();
  const getNow = () => cachedNow || (p.then(() => cachedNow = 0), cachedNow = Date.now());
  function createInvoker(initialValue, instance) {
    const invoker = (e) => {
      if (!e._vts) {
        e._vts = Date.now();
      } else if (e._vts <= invoker.attached) {
        return;
      }
      const value = invoker.value;
      if (isArray(value)) {
        const originalStop = e.stopImmediatePropagation;
        e.stopImmediatePropagation = () => {
          originalStop.call(e);
          e._stopped = true;
        };
        const handlers = value.slice();
        const args = [e];
        for (let i = 0; i < handlers.length; i++) {
          if (e._stopped) {
            break;
          }
          const handler = handlers[i];
          if (handler) {
            callWithAsyncErrorHandling(
              handler,
              instance,
              5,
              args
            );
          }
        }
      } else {
        callWithAsyncErrorHandling(
          value,
          instance,
          5,
          [e]
        );
      }
    };
    invoker.value = initialValue;
    invoker.attached = getNow();
    return invoker;
  }
  const isNativeOn = (key) => key.charCodeAt(0) === 111 && key.charCodeAt(1) === 110 && // lowercase letter
  key.charCodeAt(2) > 96 && key.charCodeAt(2) < 123;
  const patchProp = (el2, key, prevValue, nextValue, namespace, parentComponent) => {
    const isSVG = namespace === "svg";
    if (key === "class") {
      patchClass(el2, nextValue, isSVG);
    } else if (key === "style") {
      patchStyle(el2, prevValue, nextValue);
    } else if (isOn(key)) {
      if (!isModelListener(key)) {
        patchEvent(el2, key, prevValue, nextValue, parentComponent);
      }
    } else if (key[0] === "." ? (key = key.slice(1), true) : key[0] === "^" ? (key = key.slice(1), false) : shouldSetAsProp(el2, key, nextValue, isSVG)) {
      patchDOMProp(el2, key, nextValue);
      if (!el2.tagName.includes("-") && (key === "value" || key === "checked" || key === "selected")) {
        patchAttr(el2, key, nextValue, isSVG, parentComponent, key !== "value");
      }
    } else if (
      // #11081 force set props for possible async custom element
      el2._isVueCE && // #12408 check if it's declared prop or it's async custom element
      (shouldSetAsPropForVueCE(el2, key) || // @ts-expect-error _def is private
      el2._def.__asyncLoader && (/[A-Z]/.test(key) || !isString(nextValue)))
    ) {
      patchDOMProp(el2, camelize(key), nextValue, parentComponent, key);
    } else {
      if (key === "true-value") {
        el2._trueValue = nextValue;
      } else if (key === "false-value") {
        el2._falseValue = nextValue;
      }
      patchAttr(el2, key, nextValue, isSVG);
    }
  };
  function shouldSetAsProp(el2, key, value, isSVG) {
    if (isSVG) {
      if (key === "innerHTML" || key === "textContent") {
        return true;
      }
      if (key in el2 && isNativeOn(key) && isFunction(value)) {
        return true;
      }
      return false;
    }
    if (key === "spellcheck" || key === "draggable" || key === "translate" || key === "autocorrect") {
      return false;
    }
    if (key === "sandbox" && el2.tagName === "IFRAME") {
      return false;
    }
    if (key === "form") {
      return false;
    }
    if (key === "list" && el2.tagName === "INPUT") {
      return false;
    }
    if (key === "type" && el2.tagName === "TEXTAREA") {
      return false;
    }
    if (key === "width" || key === "height") {
      const tag = el2.tagName;
      if (tag === "IMG" || tag === "VIDEO" || tag === "CANVAS" || tag === "SOURCE") {
        return false;
      }
    }
    if (isNativeOn(key) && isString(value)) {
      return false;
    }
    return key in el2;
  }
  function shouldSetAsPropForVueCE(el2, key) {
    const props = (
      // @ts-expect-error _def is private
      el2._def.props
    );
    if (!props) {
      return false;
    }
    const camelKey = camelize(key);
    return Array.isArray(props) ? props.some((prop) => camelize(prop) === camelKey) : Object.keys(props).some((prop) => camelize(prop) === camelKey);
  }
  const getModelAssigner = (vnode) => {
    const fn = vnode.props["onUpdate:modelValue"] || false;
    return isArray(fn) ? (value) => invokeArrayFns(fn, value) : fn;
  };
  function onCompositionStart(e) {
    e.target.composing = true;
  }
  function onCompositionEnd(e) {
    const target = e.target;
    if (target.composing) {
      target.composing = false;
      target.dispatchEvent(new Event("input"));
    }
  }
  const assignKey = /* @__PURE__ */ Symbol("_assign");
  const initialValueKey = /* @__PURE__ */ Symbol("_initialValue");
  function castValue(value, trim, number) {
    if (trim) value = value.trim();
    if (number) value = looseToNumber(value);
    return value;
  }
  const vModelText = {
    created(el2, { modifiers: { lazy, trim, number } }, vnode) {
      if (el2.parentNode) {
        if (el2.type === "text") {
          el2[initialValueKey] = el2.defaultValue.replace(/[\r\n]/g, "");
        } else if (el2.type === "textarea") {
          el2[initialValueKey] = el2.defaultValue.replace(/\r\n?/g, "\n");
        }
      }
      el2[assignKey] = getModelAssigner(vnode);
      const castToNumber = number || vnode.props && vnode.props.type === "number";
      addEventListener(el2, lazy ? "change" : "input", (e) => {
        if (e.target.composing) return;
        el2[assignKey](castValue(el2.value, trim, castToNumber));
      });
      if (trim || castToNumber) {
        addEventListener(el2, "change", () => {
          el2.value = castValue(el2.value, trim, castToNumber);
        });
      }
      if (!lazy) {
        addEventListener(el2, "compositionstart", onCompositionStart);
        addEventListener(el2, "compositionend", onCompositionEnd);
        addEventListener(el2, "change", onCompositionEnd);
      }
    },
    // set value on mounted so it's after min/max for type="range"
    mounted(el2, { value, modifiers: { trim, number } }) {
      const newValue = value == null ? "" : value;
      const initialValue = el2[initialValueKey];
      delete el2[initialValueKey];
      if (initialValue !== void 0 && (el2.type === "text" || el2.type === "textarea") && el2.value !== initialValue) {
        el2[assignKey](castValue(el2.value, trim, number));
      } else {
        el2.value = newValue;
      }
    },
    beforeUpdate(el2, { value, oldValue, modifiers: { lazy, trim, number } }, vnode) {
      el2[assignKey] = getModelAssigner(vnode);
      if (el2.composing) return;
      const elValue = (number || el2.type === "number") && !/^0\d/.test(el2.value) ? looseToNumber(el2.value) : el2.value;
      const newValue = value == null ? "" : value;
      if (elValue === newValue) {
        return;
      }
      const rootNode = el2.getRootNode();
      if ((rootNode instanceof Document || rootNode instanceof ShadowRoot) && rootNode.activeElement === el2 && el2.type !== "range") {
        if (lazy && value === oldValue) {
          return;
        }
        if (trim && el2.value.trim() === newValue) {
          return;
        }
      }
      el2.value = newValue;
    }
  };
  const systemModifiers = ["ctrl", "shift", "alt", "meta"];
  const modifierGuards = {
    stop: (e) => e.stopPropagation(),
    prevent: (e) => e.preventDefault(),
    self: (e) => e.target !== e.currentTarget,
    ctrl: (e) => !e.ctrlKey,
    shift: (e) => !e.shiftKey,
    alt: (e) => !e.altKey,
    meta: (e) => !e.metaKey,
    left: (e) => "button" in e && e.button !== 0,
    middle: (e) => "button" in e && e.button !== 1,
    right: (e) => "button" in e && e.button !== 2,
    exact: (e, modifiers) => systemModifiers.some((m) => e[`${m}Key`] && !modifiers.includes(m))
  };
  const withModifiers = (fn, modifiers) => {
    if (!fn) return fn;
    const cache = fn._withMods || (fn._withMods = {});
    const cacheKey = modifiers.join(".");
    return cache[cacheKey] || (cache[cacheKey] = ((event, ...args) => {
      for (let i = 0; i < modifiers.length; i++) {
        const guard = modifierGuards[modifiers[i]];
        if (guard && guard(event, modifiers)) return;
      }
      return fn(event, ...args);
    }));
  };
  const rendererOptions = /* @__PURE__ */ extend({ patchProp }, nodeOps);
  let renderer;
  function ensureRenderer() {
    return renderer || (renderer = createRenderer(rendererOptions));
  }
  const createApp = ((...args) => {
    const app = ensureRenderer().createApp(...args);
    const { mount } = app;
    app.mount = (containerOrSelector) => {
      const container = normalizeContainer(containerOrSelector);
      if (!container) return;
      const component = app._component;
      if (!isFunction(component) && !component.render && !component.template) {
        component.template = container.innerHTML;
      }
      if (container.nodeType === 1) {
        container.textContent = "";
      }
      const proxy = mount(container, false, resolveRootNamespace(container));
      if (container instanceof Element) {
        container.removeAttribute("v-cloak");
        container.setAttribute("data-v-app", "");
      }
      return proxy;
    };
    return app;
  });
  function resolveRootNamespace(container) {
    if (container instanceof SVGElement) {
      return "svg";
    }
    if (typeof MathMLElement === "function" && container instanceof MathMLElement) {
      return "mathml";
    }
  }
  function normalizeContainer(container) {
    if (isString(container)) {
      const res = document.querySelector(container);
      return res;
    }
    return container;
  }
  const POLL_MS = 2e3;
  const LONG_TEXT_LINES = 20;
  const SYSTEM_PREVIEW_LINES = 8;
  const STORE_PREFIX = "dsh.sessionview.";
  const SIDEBAR_AUTO_COLLAPSE = 1024;
  const RAIL_W = 56;
  const SIDEBAR_DEFAULT = 280;
  const SIDEBAR_MIN = 264;
  const SIDEBAR_MAX = 420;
  const DETAILS_DEFAULT = 400;
  const DETAILS_MIN = 300;
  const DETAILS_MAX = 520;
  const CENTER_MIN = 640;
  function storeGet(key) {
    try {
      return window.localStorage.getItem(STORE_PREFIX + key);
    } catch {
      return null;
    }
  }
  function storeSet(key, value) {
    try {
      window.localStorage.setItem(STORE_PREFIX + key, value);
    } catch {
    }
  }
  function storeJSON(key, fallback) {
    const raw = storeGet(key);
    if (!raw) return fallback;
    try {
      const v = JSON.parse(raw);
      return v && typeof v === "object" ? v : fallback;
    } catch {
      return fallback;
    }
  }
  function fmtSize(bytes) {
    if (!bytes && bytes !== 0) return "—";
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1024 / 1024).toFixed(2) + " MB";
  }
  function fmtClock(value) {
    const d = new Date(value);
    if (isNaN(d.getTime())) return "—";
    const pad = (n) => (n < 10 ? "0" : "") + n;
    return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + " " + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds());
  }
  function relTime(value) {
    const t = new Date(value).getTime();
    if (isNaN(t)) return "—";
    const secs = Math.round((Date.now() - t) / 1e3);
    if (secs < 5) return "刚刚";
    if (secs < 60) return secs + " 秒前";
    if (secs < 3600) return Math.floor(secs / 60) + " 分钟前";
    if (secs < 86400) return Math.floor(secs / 3600) + " 小时前";
    return Math.floor(secs / 86400) + " 天前";
  }
  const anchors = /* @__PURE__ */ new Map();
  function registerAnchor(n, el2) {
    anchors.set(n, el2);
  }
  function unregisterAnchor(n, el2) {
    if (anchors.get(n) === el2) anchors.delete(n);
  }
  function anchorOf(n) {
    return anchors.get(n) || null;
  }
  function clearAnchors() {
    anchors.clear();
  }
  const state = /* @__PURE__ */ reactive({
    root: "",
    generated: "",
    sessions: [],
    current: null,
    lines: [],
    nextFrom: 0,
    curSize: -1,
    curMtime: 0,
    filter: "",
    follow: true,
    // 旧页 live 模式默认开
    forceCollapse: false,
    onlyTools: false,
    callEst: {},
    imgAttr: null,
    pullError: "",
    polling: false,
    theme: "light",
    view: "chat",
    markdown: true,
    unit: "token",
    sidebar: SIDEBAR_DEFAULT,
    details: 0,
    narrowExpanded: false,
    narrow: false,
    collapsed: {},
    overflow: {},
    listSig: "",
    meta: null,
    trajKinds: {},
    trajOpen: {},
    // P9：轨迹页选中的行（transcript 行号）。选中后右侧详情栏顶部显示该步的
    // 完整输入/输出/图片；再点同一行取消；跳对话按钮仍走 jumpToLine。
    trajSelected: null
  });
  function clampWidth(px, min, max) {
    return Math.min(max, Math.max(min, Math.round(px)));
  }
  function computeColumns(viewport, sidebar, details) {
    const s = sidebar === 0 ? RAIL_W : clampWidth(sidebar, SIDEBAR_MIN, SIDEBAR_MAX);
    const d0 = details === 0 ? 0 : clampWidth(details, DETAILS_MIN, DETAILS_MAX);
    if (s + d0 + CENTER_MIN <= viewport) {
      return { sidebar: s, center: viewport - s - d0, details: d0 };
    }
    const d1 = d0 === 0 ? 0 : Math.max(DETAILS_MIN, viewport - s - CENTER_MIN);
    if (s + d1 + CENTER_MIN <= viewport) {
      return { sidebar: s, center: CENTER_MIN, details: d1 };
    }
    return { sidebar: s, center: Math.max(0, viewport - s), details: 0 };
  }
  const layout = /* @__PURE__ */ reactive({
    viewport: 0,
    cols: { sidebar: SIDEBAR_DEFAULT, center: 0, details: 0 },
    sidebarCollapsed: false,
    detailsCollapsed: true
  });
  function applyLayout() {
    const viewport = layout.viewport;
    state.narrow = viewport < SIDEBAR_AUTO_COLLAPSE;
    const sidebarCollapsed = state.narrow ? !state.narrowExpanded : state.sidebar === 0;
    const sidebarPref = sidebarCollapsed ? 0 : state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar;
    layout.cols = computeColumns(viewport, sidebarPref, state.details);
    layout.sidebarCollapsed = sidebarCollapsed;
    layout.detailsCollapsed = layout.cols.details === 0;
  }
  function sidebarCollapsedNow() {
    return state.narrow ? !state.narrowExpanded : state.sidebar === 0;
  }
  function toggleSidebar() {
    if (state.narrow) {
      state.narrowExpanded = !state.narrowExpanded;
    } else {
      state.sidebar = state.sidebar === 0 ? SIDEBAR_DEFAULT : 0;
    }
    persistLayout();
    applyLayout();
  }
  function toggleDetails() {
    if (state.details > 0) {
      state.details = 0;
    } else {
      state.details = clampWidth(state.details || DETAILS_DEFAULT, DETAILS_MIN, DETAILS_MAX);
      const viewport = layout.viewport;
      const collapsed = sidebarCollapsedNow();
      const pref = collapsed ? 0 : state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar;
      if (computeColumns(viewport, pref, state.details).details === 0 && !collapsed) {
        if (state.narrow) {
          state.narrowExpanded = false;
        } else {
          state.sidebar = 0;
        }
      }
    }
    persistLayout();
    applyLayout();
  }
  function openDetails() {
    if (state.details === 0) toggleDetails();
  }
  function persistLayout() {
    storeSet("layout.sidebar", String(state.sidebar));
    storeSet("layout.details", String(state.details));
    storeSet("layout.narrowExpanded", state.narrowExpanded ? "1" : "0");
  }
  function loadState() {
    state.collapsed = storeJSON("collapsed", {});
    state.overflow = storeJSON("overflow", {});
    state.markdown = storeGet("markdown") !== "0";
    state.unit = storeGet("unit") === "char" ? "char" : "token";
    const rawSidebar = storeGet("layout.sidebar");
    if (rawSidebar !== null) {
      const sidebar = Number(rawSidebar);
      if (!isNaN(sidebar) && sidebar >= 0) state.sidebar = sidebar;
    }
    const rawDetails = storeGet("layout.details");
    const details = rawDetails === null ? 0 : Number(rawDetails);
    state.details = !isNaN(details) && details > 0 ? details : 0;
    state.narrowExpanded = storeGet("layout.narrowExpanded") === "1";
  }
  const THEME_KEY = "theme";
  const DEFAULT_THEME = "light";
  function applyTheme(theme) {
    state.theme = theme === "dark" ? "dark" : DEFAULT_THEME;
    document.documentElement.setAttribute("data-theme", state.theme);
  }
  function storedTheme() {
    const saved = storeGet(THEME_KEY);
    return saved === "dark" || saved === "light" ? saved : DEFAULT_THEME;
  }
  function toggleTheme() {
    const next = state.theme === "dark" ? "light" : "dark";
    applyTheme(next);
    storeSet(THEME_KEY, next);
  }
  function switchView(view) {
    state.view = view === "trajectory" ? "trajectory" : "chat";
  }
  const lightbox = /* @__PURE__ */ reactive({ open: false, url: "", ref: "", scale: 1, tx: 0, ty: 0 });
  function openLightbox(url, ref2) {
    lightbox.open = true;
    lightbox.url = url;
    lightbox.ref = ref2;
    lightbox.scale = 1;
    lightbox.tx = 0;
    lightbox.ty = 0;
  }
  function closeLightbox() {
    lightbox.open = false;
    lightbox.url = "";
    lightbox.ref = "";
    lightbox.scale = 1;
    lightbox.tx = 0;
    lightbox.ty = 0;
  }
  function zoomLightbox(factor, mx, my, rect) {
    const s0 = lightbox.scale;
    const s1 = Math.min(8, Math.max(0.15, s0 * factor));
    if (s1 === s0) return;
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const px = (mx - cx - lightbox.tx) / s0;
    const py = (my - cy - lightbox.ty) / s0;
    lightbox.tx += px * (s0 - s1);
    lightbox.ty += py * (s0 - s1);
    lightbox.scale = s1;
  }
  function resetLightbox() {
    lightbox.scale = 1;
    lightbox.tx = 0;
    lightbox.ty = 0;
  }
  function setBannerText(text) {
    state.pullError = text;
  }
  const ROOT_PROJECT = "（根目录）";
  function projectOf(id) {
    const i = String(id || "").indexOf("/");
    return i <= 0 ? ROOT_PROJECT : String(id).slice(0, i);
  }
  function subPathOf(id) {
    const parts = String(id || "").split("/");
    parts.pop();
    parts.shift();
    return parts.join("/");
  }
  function shortSHA(value) {
    const s = String(value || "");
    if (!s) return "—";
    return s.length > 12 ? s.slice(0, 12) : s;
  }
  function firstLine(text, limit) {
    const s = String(text === void 0 || text === null ? "" : text);
    const lines = s.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const t = lines[i].replace(/\s+/g, " ").trim();
      if (t) {
        const max = limit || 96;
        return t.length > max ? t.slice(0, max) + "…" : t;
      }
    }
    return "";
  }
  function normalizeLine(line) {
    if (line && line.reasoning === void 0 && line.reasoning_content !== void 0) {
      line.reasoning = line.reasoning_content;
    }
    return line;
  }
  function normalizeLines(lines) {
    return (lines || []).map(normalizeLine);
  }
  function fmtTokens(n) {
    const v = Number(n) || 0;
    if (v >= 1e6) return (v / 1e6).toFixed(2) + "M";
    if (v >= 1e3) return (v / 1e3).toFixed(v >= 1e4 ? 0 : 1) + "k";
    return String(v);
  }
  function fmtDur(ms) {
    const v = Number(ms) || 0;
    if (v < 1e3) return Math.round(v) + "ms";
    if (v < 6e4) return (v / 1e3).toFixed(1) + "s";
    const m = Math.floor(v / 6e4);
    const sec = Math.round(v % 6e4 / 1e3);
    return m + "m" + (sec < 10 ? "0" : "") + sec + "s";
  }
  function countText(chars, tokens) {
    if (state.unit === "char") return (Number(chars) || 0) + " 字符";
    return "≈ " + fmtTokens(tokens) + " tokens";
  }
  function countValue(chars, tokens) {
    if (state.unit === "char") return String(Number(chars) || 0);
    return "≈ " + fmtTokens(tokens);
  }
  function unitLabel() {
    return state.unit === "char" ? "字符" : "tokens";
  }
  function usageLines(lines) {
    const out = [];
    (lines || []).forEach((l) => {
      if (l && l.t === "usage" && l.stats) out.push(l);
    });
    return out;
  }
  function aggregate(usages) {
    if (!usages.length) return null;
    const agg = {
      requests: 0,
      promptTokens: 0,
      cachedTokens: 0,
      completionTokens: 0,
      reasoningTokens: 0,
      durationMs: 0,
      ttftMs: 0,
      genMs: 0,
      streamed: false,
      firstTs: "",
      lastTs: "",
      perRequest: usages
    };
    usages.forEach((l) => {
      const st = l.stats;
      agg.requests += 1;
      agg.promptTokens += Number(st.promptTokens) || 0;
      agg.cachedTokens += Number(st.cachedTokens) || 0;
      agg.completionTokens += Number(st.completionTokens) || 0;
      agg.reasoningTokens += Number(st.reasoningTokens) || 0;
      const dur = Number(st.durationMs) || 0;
      const ttft = Number(st.ttftMs) || 0;
      agg.durationMs += dur;
      agg.ttftMs += ttft;
      agg.genMs += Math.max(dur - ttft, 1);
      if (st.streamed) agg.streamed = true;
      const ts = l.ts || "";
      if (ts && (!agg.firstTs || ts < agg.firstTs)) agg.firstTs = ts;
      if (ts && ts > agg.lastTs) agg.lastTs = ts;
    });
    agg.cacheHitPct = agg.promptTokens ? agg.cachedTokens * 100 / agg.promptTokens : 0;
    agg.avgTtftMs = agg.ttftMs / agg.requests;
    agg.avgDurationMs = agg.durationMs / agg.requests;
    agg.outputTps = agg.genMs ? agg.completionTokens * 1e3 / agg.genMs : 0;
    agg.spanMs = 0;
    if (agg.firstTs && agg.lastTs) {
      const t0 = Date.parse(agg.firstTs);
      const t1 = Date.parse(agg.lastTs);
      if (!isNaN(t0) && !isNaN(t1) && t1 > t0) agg.spanMs = t1 - t0;
    }
    return agg;
  }
  function scanStats(session) {
    const st = session && session.stats;
    if (!st || !st.requests) return null;
    return st;
  }
  function fmtCost(cost, unpriced) {
    if (!cost) return "";
    return "" + (cost.currency || "") + cost.total.toFixed(2);
  }
  function statsSummary(st, session) {
    if (!st) return "";
    const parts = ["输入 " + fmtTokens(st.promptTokens) + " · 输出 " + fmtTokens(st.completionTokens)];
    if (st.promptTokens) parts.push("缓存 " + st.cacheHitPct.toFixed(0) + "%");
    if (st.avgTtftMs) parts.push("首字 " + fmtDur(st.avgTtftMs));
    const money = session ? fmtCost(session.cost) : "";
    if (money) parts.push(money);
    return parts.join(" · ");
  }
  function indexCallEstimates(lines) {
    (lines || []).forEach((line) => {
      const est = line.est || {};
      (line.tool_calls || []).forEach((c, i) => {
        if (c && c.id) state.callEst[c.id] = est.calls && est.calls[i] || 0;
      });
    });
  }
  function estOf(line) {
    const e = line && line.est;
    return {
      text: e && e.text || 0,
      reasoning: e && e.reasoning || 0,
      calls: e && e.calls || [],
      images: e && e.images || 0,
      imageCount: e && e.imageCount || 0
    };
  }
  const STAGE_ORDER = ["vector", "style", "chapters", "convert", "checker", "style-fix", "figure-check"];
  function stageRank(stage) {
    const i = STAGE_ORDER.indexOf(stage);
    return i < 0 ? STAGE_ORDER.length : i;
  }
  function stageTitleOf(stage) {
    const map = {
      vector: "矢量图",
      style: "样式",
      chapters: "章节划分",
      convert: "章节转换",
      checker: "章节核对",
      "style-fix": "样式修复",
      "figure-check": "逐图校验"
    };
    return map[stage] || stage || "其他会话";
  }
  function progressKeyFor(stage) {
    if (stage === "vector") return "images";
    if (stage === "checker" || stage === "style-fix") return "convert";
    return stage;
  }
  function stageStatusText(stages, stage, live) {
    if (live) return { text: "运行中", state: "running", title: "这一阶段还有 " + live + " 个会话在实时写入" };
    if (!stages) return null;
    const key = progressKeyFor(stage);
    const v = stages[key];
    if (v === void 0 || v === null || v === "") {
      return { text: "未开始", state: "idle", title: "progress.json: " + key + " 还没有记录" };
    }
    const text = String(v);
    const done = /^(done|ok|true|finished|complete[d]?)$/i.test(text);
    if (done) return { text: "✓ 完成", state: "done", title: "progress.json: " + key + " = " + text };
    return { text, state: "busy", title: "progress.json: " + key + " = " + text };
  }
  function projectProgressLine(stages) {
    if (!stages) return null;
    const order = ["images", "style", "chapters", "convert", "assemble"];
    const parts = [];
    order.forEach((k) => {
      if (stages[k] === void 0) return;
      const done = /^(done|ok|true|finished|complete[d]?)$/i.test(String(stages[k]));
      parts.push(k + (done ? " ✓" : " " + stages[k]));
    });
    if (!parts.length) return null;
    return parts.join(" · ");
  }
  function sessionHaystack(s) {
    const parts = [
      s.title,
      s.label,
      s.id,
      s.name,
      s.project,
      s.stage,
      s.stageTitle,
      s.imageName,
      s.imageType,
      s.imageCaption,
      s.imageShort,
      s.imageFile,
      s.imageLabel
    ];
    if (s.page) parts.push("p" + s.page, "p." + s.page, "页" + s.page, String(s.page));
    if (s.imageOrder) parts.push("#" + s.imageOrder, "第" + s.imageOrder + "张", String(s.imageOrder));
    if (s.chapterOrder) parts.push("第" + s.chapterOrder + "章", "chapter " + s.chapterOrder, String(s.chapterOrder));
    if (s.endState === "error") parts.push("错误", "未提交", "error");
    if (s.endState === "done") parts.push("已提交", "完成", "done");
    return parts.filter(Boolean).join(" ").toLowerCase();
  }
  function imageDisplayName(s) {
    return s.imageCaption || s.imageLabel || s.imageShort || s.imageName;
  }
  function sessionTitleOf(s) {
    if (s.imageName && (s.page || s.imageOrder || s.imageShort || s.imageLabel || s.imageCaption)) {
      return (s.stageTitle || "矢量图") + " · " + imageDisplayName(s);
    }
    return s.title || s.label || s.name;
  }
  function sessionTip(s) {
    const tip = [sessionTitleOf(s), s.id];
    const st = scanStats(s);
    if (st) {
      tip.push(statsSummary(st, s));
      tip.push(st.requests + " 次 API 请求 · 输入 " + st.promptTokens + " tokens（其中 " + st.cachedTokens + " 命中前缀缓存）· 输出 " + st.completionTokens + (st.reasoningTokens ? "（思考 " + st.reasoningTokens + "）" : "") + " · 平均耗时 " + fmtDur(st.avgDurationMs) + " · 平均首字 " + fmtDur(st.avgTtftMs) + " · 输出 " + (st.outputTps || 0).toFixed(1) + " tok/s");
    }
    const m = metaOf(s);
    if (m) {
      tip.push("含系统提示词快照：" + m.count + " 条 t=meta 元信息行，最新一条 " + countText(m.promptChars || 0, m.promptTokenEst) + (m.model ? "（模型 " + m.model + "）" : "") + (m.tools ? "，含 " + m.tools + " 个工具定义" : ""));
    }
    if (s.imageName) {
      tip.push("来源图片: " + imageDisplayName(s) + (s.imageType ? "（" + s.imageType + "）" : "") + (s.page ? "\n第 " + s.page + " 页" : "") + (s.imageOrder ? "\n书内第 " + s.imageOrder + " 张" : "") + (s.imageCaption ? "\n图注: " + s.imageCaption : ""));
      if (s.imageFile) tip.push("图片文件: " + s.imageFile);
      if (s.imageName) tip.push("图片哈希: " + s.imageName);
      if (s.imagePath) tip.push("图片路径: " + s.imagePath);
    }
    if (s.chapterOrder) tip.push("第 " + s.chapterOrder + " 章");
    if (s.live) tip.push("状态: 运行中（正在实时写入）");
    else if (s.endState === "done") tip.push("状态: 已提交（正常结束）");
    else if (s.endState === "error") tip.push("状态: 错误终止（有过工具回执但从没提交成功）");
    else tip.push("状态: 未开始（还没有任何工具回执）");
    tip.push("消息 " + s.messages + " 条 · " + fmtSize(s.size) + " · 最后写入 " + fmtClock(s.mtime));
    const sub = subPathOf(s.id);
    if (sub) tip.push("目录 " + sub);
    return tip.join("\n");
  }
  function metaOf(session) {
    return session && session.meta && session.meta.count ? session.meta : null;
  }
  function usageChipText(s) {
    const st = scanStats(s);
    return st ? fmtTokens(st.promptTokens) + " / " + fmtTokens(st.completionTokens) : "";
  }
  function imageChipText(s) {
    const bits = [];
    if (s.page) bits.push("p" + s.page);
    if (s.imageOrder) bits.push("#" + s.imageOrder);
    if (s.imageType) bits.push(s.imageType);
    return bits.join("·");
  }
  function imageTipText(s) {
    const bits = [imageDisplayName(s)];
    if (s.imageFile && s.imageFile !== imageDisplayName(s)) bits.push("文件 " + s.imageFile);
    if (s.imageName) bits.push("哈希 " + s.imageName);
    if (s.imagePath) bits.push(s.imagePath);
    if (s.page) bits.push("第 " + s.page + " 页");
    if (s.imageOrder) bits.push("书内第 " + s.imageOrder + " 张");
    if (s.imageType) bits.push(s.imageType);
    return bits.join("\n");
  }
  function buildGroups() {
    const q = state.filter;
    let groups = [];
    const byName = {};
    state.sessions.forEach((s) => {
      const name = s.project || projectOf(s.id);
      let g = byName[name];
      if (!g) {
        const cut = name.lastIndexOf("/");
        g = byName[name] = {
          name,
          prefix: cut > 0 ? name.slice(0, cut + 1) : "",
          title: cut > 0 ? name.slice(cut + 1) : name,
          legacy: false,
          items: [],
          live: 0,
          matched: false,
          stages: {}
        };
        groups.push(g);
      }
      if (s.projectLegacy && name === "（根目录）") g.legacy = true;
      if (q && sessionHaystack(s).indexOf(q) < 0) return;
      g.items.push(s);
      if (s.live) g.live++;
      const stage = s.stage || "session";
      let sg = g.stages[stage];
      if (!sg) sg = g.stages[stage] = { stage, title: stageTitleOf(stage), items: [], live: 0 };
      sg.items.push(s);
      if (s.live) sg.live++;
    });
    if (q) {
      groups = groups.filter((g) => g.items.length > 0);
      groups.forEach((g) => {
        g.matched = true;
      });
    }
    return groups;
  }
  function findSession(id) {
    let found = null;
    state.sessions.forEach((s) => {
      if (s.id === id) found = s;
    });
    return found;
  }
  function groupKey(kind, name) {
    return kind + ":" + name;
  }
  function isCollapsed(key) {
    return state.collapsed[key] === true;
  }
  function hasCollapseMemory(key) {
    return Object.prototype.hasOwnProperty.call(state.collapsed, key);
  }
  function setCollapsed(key, collapsed) {
    state.collapsed[key] = !!collapsed;
    storeSet("collapsed", JSON.stringify(state.collapsed));
  }
  function groupWantOpen(key, holdsCurrent) {
    if (hasCollapseMemory(key)) return !isCollapsed(key);
    return !!holdsCurrent;
  }
  function setOverflowOpen(key, open) {
    state.overflow[key] = true;
    storeSet("overflow", JSON.stringify(state.overflow));
  }
  let pullSeq = 0;
  function revealProject(project) {
    const wraps = document.querySelectorAll(".proj-group");
    let hit = null;
    wraps.forEach((w) => {
      const el2 = w;
      const summary = el2.querySelector(".proj-row");
      if (!hit && summary && summary.title === project) hit = el2;
    });
    const target = hit;
    if (!target) return;
    if (!target.open) {
      target.open = true;
      target.dataset.open = "1";
      setCollapsed(groupKey("proj", project), false);
    }
    target.scrollIntoView({ block: "nearest" });
  }
  function applyIndex(payload) {
    state.sessions = payload.sessions || [];
    state.root = payload.root || state.root;
    state.generated = payload.generated || state.generated;
    state.listSig = listSigOf();
  }
  function listSigOf() {
    const parts = [state.filter, state.sessions.length];
    state.sessions.forEach((s) => {
      parts.push([
        s.id,
        s.messages,
        s.cost ? s.cost.total + (s.cost.currency || "") : "",
        s.stats ? s.stats.requests : 0,
        s.projectStages ? JSON.stringify(s.projectStages) : ""
      ].join("~"));
    });
    return parts.join("|");
  }
  function refreshIndex() {
    return fetch("/api/index", { cache: "no-store" }).then((res) => {
      if (!res.ok) throw new Error("HTTP " + res.status);
      return res.json();
    }).then((payload) => {
      applyIndex(payload);
      state.polling = true;
      const current = findSession(state.current ? state.current.id : "");
      if (!current) return;
      state.current = current;
      const mtime = new Date(current.mtime).getTime();
      const changed = current.size !== state.curSize || mtime !== state.curMtime;
      if (changed && state.curSize >= 0) {
        state.curMtime = mtime;
        return pullSession(false);
      }
      state.curSize = current.size;
      state.curMtime = mtime;
    }).catch(() => {
      state.polling = false;
    });
  }
  function pullSession(reset) {
    const s = state.current;
    if (!s) return Promise.resolve();
    const from = reset ? 0 : state.nextFrom;
    const url = "/api/session?id=" + encodeURIComponent(s.id) + "&from=" + from;
    const seq = ++pullSeq;
    return fetch(url, { cache: "no-store" }).then((res) => {
      if (!res.ok) throw new Error("HTTP " + res.status);
      return res.json();
    }).then((payload) => {
      if (seq !== pullSeq) return;
      if (!reset && payload.nextFrom < state.nextFrom) {
        state.lines = [];
        state.nextFrom = 0;
        return pullSession(true);
      }
      state.nextFrom = payload.nextFrom;
      state.curSize = payload.size;
      if (reset) {
        state.lines = normalizeLines(payload.lines);
        indexCallEstimates(state.lines);
      } else {
        const fresh = normalizeLines(payload.lines);
        indexCallEstimates(fresh);
        fresh.forEach((line) => {
          state.lines.push(line);
        });
      }
    }).catch((err) => {
      setBannerText("拉取会话失败：" + err.message);
    });
  }
  function selectSession(id) {
    const s = findSession(id);
    state.current = s;
    state.lines = [];
    state.callEst = {};
    state.nextFrom = 0;
    state.curSize = -1;
    state.curMtime = 0;
    state.meta = null;
    state.trajOpen = {};
    if (!s) return;
    void pullSession(true);
  }
  function bootData() {
    void refreshIndex().then(() => {
      if (state.sessions.length && !state.current) {
        let live = null;
        state.sessions.forEach((s) => {
          if (!live && s.live) live = s;
        });
        selectSession((live || state.sessions[0]).id);
      }
    });
    window.setInterval(() => {
      if (!document.hidden) void refreshIndex();
    }, POLL_MS);
  }
  function el$1(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== void 0 && text !== null) node.textContent = text;
    return node;
  }
  function fmtChars(n) {
    if (!n) return "0 字符";
    if (n < 1024) return n + " 字符";
    return (n / 1024).toFixed(1) + " KB";
  }
  function prettyJSON(raw) {
    if (typeof raw !== "string" || !raw.trim()) return "";
    try {
      return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
      return raw;
    }
  }
  const JSON_HL_MAX_CHARS = 200 * 1024;
  const JSON_HL_MAX_LINES = 4e3;
  const IO_FOLD_LINES = 16;
  function jsonPretty(raw) {
    const s = String(raw === void 0 || raw === null ? "" : raw).trim();
    if (!s || s.charAt(0) !== "{" && s.charAt(0) !== "[") return null;
    try {
      return JSON.stringify(JSON.parse(s), null, 2);
    } catch {
      return null;
    }
  }
  function countLines(text) {
    return String(text || "").split("\n").length;
  }
  function jsonTooBig(text) {
    return text.length > JSON_HL_MAX_CHARS || countLines(text) > JSON_HL_MAX_LINES;
  }
  function appendJSONSpans(parent, text) {
    const re = /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)/g;
    let last = 0;
    let m;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) parent.appendChild(document.createTextNode(text.slice(last, m.index)));
      const cls = m[1] !== void 0 ? "k" : m[2] !== void 0 ? "s" : m[3] !== void 0 ? "b" : "n";
      parent.appendChild(el$1("span", cls, m[0]));
      last = m.index + m[0].length;
    }
    if (last < text.length) parent.appendChild(document.createTextNode(text.slice(last)));
  }
  function consoleLineClass(line) {
    if (/^\s*(\$|#)\s+\S/.test(line)) return "cmd";
    if (/^\+\+\+|^---\s/.test(line)) return "diffhead";
    if (/^@@/.test(line)) return "hunk";
    if (/^\+/.test(line)) return "add";
    if (/^-/.test(line)) return "del";
    if (/^\s*!/.test(line)) return "bad";
    if (/Overfull|Underfull/.test(line)) return "warn";
    return "";
  }
  const CONSOLE_TOKENS = /(https?:\/\/[^\s"'<>]+)|((?:\.{0,2}\/|\/)[\w.\-]+\/[\w.\-/]*[\w.\-])|(\b(?:ERROR|Error|error|FAILED|FAIL|Failure|failed|FATAL|Fatal)\b)|(\b(?:WARNING|Warning|warning|WARN|Warn|OVERFULL|Overfull|UNDERFULL|Underfull)\b)|(\b(?:OK|PASS|PASSED|COMPILE OK|SUCCESS|Success|done)\b)/g;
  function appendConsoleLine(parent, line) {
    const whole = consoleLineClass(line);
    if (whole) {
      parent.appendChild(el$1("span", whole, line));
      return;
    }
    let last = 0;
    let m;
    CONSOLE_TOKENS.lastIndex = 0;
    while ((m = CONSOLE_TOKENS.exec(line)) !== null) {
      if (m.index > last) parent.appendChild(document.createTextNode(line.slice(last, m.index)));
      const cls = m[1] || m[2] ? "path" : m[3] ? "bad" : m[4] ? "warn" : "good";
      parent.appendChild(el$1("span", cls, m[0]));
      last = m.index + m[0].length;
      if (m[0] === "") break;
    }
    if (last < line.length) parent.appendChild(document.createTextNode(line.slice(last)));
  }
  function appendConsoleSpans(parent, text) {
    String(text === void 0 || text === null ? "" : text).split("\n").forEach((line, i) => {
      if (i) parent.appendChild(document.createTextNode("\n"));
      appendConsoleLine(parent, line);
    });
  }
  function highlightMachine(parent, raw) {
    const text = String(raw === void 0 || raw === null ? "" : raw);
    const pretty = jsonPretty(text);
    if (pretty === null) {
      if (jsonTooBig(text)) {
        parent.textContent = text;
        return {
          highlighted: false,
          note: "内容过大（" + fmtChars(text.length) + "），已按纯文本显示，不做高亮"
        };
      }
      appendConsoleSpans(parent, text);
      return { highlighted: true, note: "" };
    }
    if (jsonTooBig(pretty)) {
      parent.textContent = pretty;
      return {
        highlighted: false,
        note: "内容过大（" + fmtChars(pretty.length) + "），已按纯文本显示，不做 JSON 高亮"
      };
    }
    appendJSONSpans(parent, pretty);
    return { highlighted: true, note: "" };
  }
  function machineBlock(text, cls) {
    const frag = document.createDocumentFragment();
    const pre = el$1("pre", cls || "code");
    const res = highlightMachine(pre, text);
    frag.appendChild(pre);
    if (res.note) frag.appendChild(el$1("div", "note", res.note));
    return frag;
  }
  function foldLabel(expanded, lines, chars, tokens) {
    return expanded ? "收起" : "展开全文（" + lines + " 行 / " + countText(chars, tokens) + "）";
  }
  function machineScrollInto(host, text, key, extraClass, tokens) {
    const wrap = host;
    const raw = String(text === void 0 || text === null ? "" : text);
    const pretty = jsonPretty(raw);
    const body = pretty === null ? raw : pretty;
    const big = jsonTooBig(body);
    const lines = body.split("\n");
    const scroll = el$1("div", "io-scroll");
    const pre = el$1("pre", "io-text");
    let expanded = storeGet("text." + key) === "1";
    const long = lines.length > IO_FOLD_LINES;
    const paint = () => {
      if (big) pre.textContent = body;
      else if (pretty !== null) appendJSONSpans(pre, body);
      else appendConsoleSpans(pre, body);
      scroll.classList.toggle("open", expanded);
    };
    paint();
    scroll.appendChild(pre);
    wrap.appendChild(scroll);
    if (big) {
      const size = tokens ? countText(body.length, tokens) : fmtChars(body.length);
      wrap.appendChild(el$1("div", "note", "内容过大（" + size + "），按纯文本显示，不做高亮"));
    }
    if (long) {
      const toggle = el$1("button", "text-toggle");
      toggle.type = "button";
      const label = () => {
        toggle.textContent = foldLabel(expanded, lines.length, body.length, tokens);
      };
      label();
      toggle.addEventListener("click", () => {
        expanded = !expanded;
        storeSet("text." + key, expanded ? "1" : "0");
        paint();
        label();
      });
      wrap.appendChild(toggle);
    }
  }
  function copyButton(text) {
    const btn = el$1("button", "text-toggle", "复制");
    btn.type = "button";
    btn.addEventListener("click", () => {
      const done = () => {
        btn.textContent = "已复制";
        setTimeout(() => {
          btn.textContent = "复制";
        }, 1200);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, () => {
          if (fallbackCopy(text)) done();
        });
      } else if (fallbackCopy(text)) {
        done();
      }
    });
    return btn;
  }
  function fallbackCopy(text) {
    try {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      document.body.removeChild(area);
      return ok;
    } catch {
      return false;
    }
  }
  function mdFence(line) {
    const m = /^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)\s*$/.exec(line);
    return m ? { mark: m[1].charAt(0), lang: m[2] || "" } : null;
  }
  function mdListMarker(line) {
    const m = /^([ \t]*)([-*+]|\d{1,3}[.)])\s+(.*)$/.exec(line);
    if (!m) return null;
    return {
      indent: m[1].replace(/\t/g, "  ").length,
      ordered: /\d/.test(m[2]),
      text: m[3]
    };
  }
  const MD_HR = /^(?:\*\s*){3,}$|^(?:-\s*){3,}$|^(?:_\s*){3,}$/;
  const MD_HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/;
  function mdSeparatorRow(line) {
    const s = String(line || "").trim();
    if (s.indexOf("-") < 0 || s.indexOf("|") < 0) return false;
    return /^\|?[\s:|-]+\|?$/.test(s);
  }
  function mdSplitRow(line) {
    const s = String(line || "").trim().replace(/^\|/, "").replace(/\|$/, "");
    return s.split("|").map((c) => c.trim());
  }
  function mdBlockStart(line, next) {
    const t = String(line || "").trim();
    if (!t) return true;
    if (mdFence(t) || MD_HEADING.test(t) || MD_HR.test(t)) return true;
    if (/^\s{0,3}>/.test(String(line)) || mdListMarker(String(line))) return true;
    return t.indexOf("|") >= 0 && !!next && mdSeparatorRow(next) && mdSplitRow(t).length > 1;
  }
  const MATHML_NS = "http://www.w3.org/1998/Math/MathML";
  const MATH_CHARS = {
    alpha: "α",
    beta: "β",
    gamma: "γ",
    delta: "δ",
    epsilon: "ε",
    varepsilon: "ε",
    zeta: "ζ",
    eta: "η",
    theta: "θ",
    vartheta: "ϑ",
    iota: "ι",
    kappa: "κ",
    lambda: "λ",
    mu: "μ",
    nu: "ν",
    xi: "ξ",
    pi: "π",
    varpi: "ϖ",
    rho: "ρ",
    sigma: "σ",
    varsigma: "ς",
    tau: "τ",
    upsilon: "υ",
    phi: "φ",
    varphi: "ϕ",
    chi: "χ",
    psi: "ψ",
    omega: "ω",
    Gamma: "Γ",
    Delta: "Δ",
    Theta: "Θ",
    Lambda: "Λ",
    Xi: "Ξ",
    Pi: "Π",
    Sigma: "Σ",
    Upsilon: "Υ",
    Phi: "Φ",
    Psi: "Ψ",
    Omega: "Ω"
  };
  const MATH_OPS = {
    pm: "±",
    mp: "∓",
    times: "×",
    div: "÷",
    cdot: "⋅",
    ast: "∗",
    star: "⋆",
    circ: "∘",
    bullet: "∙",
    le: "≤",
    leq: "≤",
    ge: "≥",
    geq: "≥",
    ne: "≠",
    neq: "≠",
    approx: "≈",
    equiv: "≡",
    sim: "∼",
    simeq: "≃",
    cong: "≅",
    propto: "∝",
    ll: "≪",
    gg: "≫",
    "in": "∈",
    notin: "∉",
    ni: "∋",
    subset: "⊂",
    subseteq: "⊆",
    supset: "⊃",
    supseteq: "⊇",
    cup: "∪",
    cap: "∩",
    setminus: "∖",
    emptyset: "∅",
    varnothing: "∅",
    forall: "∀",
    exists: "∃",
    nexists: "∄",
    neg: "¬",
    land: "∧",
    wedge: "∧",
    lor: "∨",
    vee: "∨",
    oplus: "⊕",
    otimes: "⊗",
    perp: "⊥",
    parallel: "∥",
    angle: "∠",
    triangle: "△",
    square: "□",
    to: "→",
    rightarrow: "→",
    leftarrow: "←",
    leftrightarrow: "↔",
    Rightarrow: "⇒",
    Leftarrow: "⇐",
    Leftrightarrow: "⇔",
    mapsto: "↦",
    implies: "⟹",
    iff: "⟺",
    uparrow: "↑",
    downarrow: "↓",
    infty: "∞",
    partial: "∂",
    nabla: "∇",
    ell: "ℓ",
    hbar: "ℏ",
    imath: "ı",
    jmath: "ȷ",
    Re: "ℜ",
    Im: "ℑ",
    aleph: "ℵ",
    wp: "℘",
    prime: "′",
    dots: "…",
    ldots: "…",
    cdots: "⋯",
    vdots: "⋮",
    ddots: "⋱",
    cases: "{",
    lbrace: "{",
    rbrace: "}",
    langle: "⟨",
    rangle: "⟩",
    lceil: "⌈",
    rceil: "⌉",
    lfloor: "⌊",
    rfloor: "⌋",
    vert: "|",
    Vert: "‖",
    backslash: "\\",
    dagger: "†",
    ddagger: "‡",
    S: "§",
    therefore: "∴",
    because: "∵",
    checkmark: "✓",
    mid: "∣",
    nmid: "∤",
    bmod: "mod",
    pmod: "mod"
  };
  const MATH_LETTER_OPS = {
    sum: "∑",
    prod: "∏",
    coprod: "∐",
    int: "∫",
    iint: "∬",
    iiint: "∭",
    oint: "∮",
    bigcup: "⋃",
    bigcap: "⋂",
    bigoplus: "⨁",
    bigotimes: "⨂",
    bigvee: "⋁",
    bigwedge: "⋀",
    lim: "lim",
    limsup: "lim sup",
    liminf: "lim inf",
    sup: "sup",
    inf: "inf",
    max: "max",
    min: "min",
    det: "det",
    gcd: "gcd",
    argmax: "arg max",
    argmin: "arg min"
  };
  const MATH_FUNCS = {
    sin: 1,
    cos: 1,
    tan: 1,
    cot: 1,
    sec: 1,
    csc: 1,
    arcsin: 1,
    arccos: 1,
    arctan: 1,
    sinh: 1,
    cosh: 1,
    tanh: 1,
    coth: 1,
    log: 1,
    ln: 1,
    lg: 1,
    exp: 1,
    deg: 1,
    dim: 1,
    ker: 1,
    hom: 1,
    Pr: 1,
    sgn: 1,
    mod: 1
  };
  const MATH_BB = {
    R: "ℝ",
    N: "ℕ",
    Z: "ℤ",
    Q: "ℚ",
    C: "ℂ",
    P: "ℙ",
    H: "ℍ",
    E: "ᵓc",
    F: "ᵓd",
    A: "��",
    B: "��",
    D: "��",
    K: "ᵔ2",
    L: "ᵓe",
    M: "ᵔ4",
    S: "��",
    U: "��",
    V: "��",
    W: "��",
    X: "��",
    Y: "��"
  };
  const MATH_MATRIX_ENVS = {
    matrix: "",
    pmatrix: "()",
    bmatrix: "[]",
    vmatrix: "||",
    Bmatrix: "{}",
    cases: "{",
    aligned: "",
    align: "",
    gathered: "",
    array: "",
    split: ""
  };
  const MATH_SPACES = {
    ",": "0.167em",
    ":": "0.222em",
    ";": "0.278em",
    "!": "-0.167em",
    quad: "1em",
    qquad: "2em",
    thinspace: "0.167em",
    medspace: "0.222em",
    thickspace: "0.278em",
    negthinspace: "-0.167em",
    space: "0.333em"
  };
  const MATH_ACCENTS = {
    hat: "ˆ",
    widehat: "ˆ",
    bar: "¯",
    overline: "¯",
    vec: "⃗",
    tilde: "˜",
    widetilde: "˜",
    dot: "˙",
    ddot: "¨",
    acute: "´",
    grave: "`",
    check: "ˇ",
    breve: "˘",
    mathring: "˚"
  };
  const MATH_FONTS = {
    mathrm: "normal",
    mathbf: "bold",
    boldsymbol: "bold",
    mathit: "italic",
    mathsf: "sans-serif",
    mathtt: "monospace",
    mathcal: "script",
    mathfrak: "fraktur",
    mathbb: "double-struck",
    mathnormal: "italic"
  };
  function mathEl(tag) {
    return document.createElementNS(MATHML_NS, tag);
  }
  function mathText(tag, str) {
    const n = mathEl(tag);
    n.appendChild(document.createTextNode(String(str)));
    return n;
  }
  function mathSymbol(ch) {
    const n = mathEl("mo");
    n.appendChild(document.createTextNode(ch));
    n.setAttribute("stretchy", "false");
    return n;
  }
  function mdMathML(tex, display) {
    const src = String(tex === void 0 || tex === null ? "" : tex);
    let pos = 0;
    const root = mathEl("math");
    if (display) root.setAttribute("display", "block");
    root.setAttribute("class", display ? "md-math md-math-block" : "md-math");
    function isSpace(c) {
      return c === " " || c === "	" || c === "\n";
    }
    function skipSpaces() {
      while (pos < src.length && isSpace(src[pos])) pos += 1;
    }
    function atEndMarker() {
      return src.charAt(pos) === "\\" && /^\\end\b/.test(src.slice(pos));
    }
    function isLetter(c) {
      return c >= "a" && c <= "z" || c >= "A" && c <= "Z";
    }
    function isDigit(c) {
      return c >= "0" && c <= "9";
    }
    function readBraceText() {
      skipSpaces();
      if (src.charAt(pos) !== "{") return "";
      let depth = 0, out = "";
      const start = pos;
      for (; pos < src.length; pos += 1) {
        const c = src[pos];
        if (c === "{") {
          depth += 1;
          if (depth === 1) continue;
        } else if (c === "}") {
          depth -= 1;
          if (depth === 0) {
            pos += 1;
            return out;
          }
        }
        out += c;
      }
      pos = start;
      return "";
    }
    function readCommandName() {
      pos += 1;
      if (pos >= src.length) return "\\";
      if (isLetter(src[pos])) {
        const s = pos;
        while (pos < src.length && isLetter(src[pos])) pos += 1;
        return src.slice(s, pos);
      }
      pos += 1;
      return src[pos - 1];
    }
    function parseGroup() {
      skipSpaces();
      if (src.charAt(pos) === "{") {
        pos += 1;
        const row = parseExpr();
        skipSpaces();
        if (src.charAt(pos) === "}") pos += 1;
        return row;
      }
      return parseAtom();
    }
    function argGroup() {
      const g = parseGroup();
      if (!g) throw new Error("mdMathML: 参数组缺失");
      return g;
    }
    function attachScript(row, sup) {
      pos += 1;
      const arg = argGroup();
      let base = row.lastChild;
      if (base) row.removeChild(base);
      else base = mathEl("mrow");
      const prev = base.nodeName;
      let isBig = prev === "mo" && base.getAttribute("largeop") === "true";
      if (!isBig && (prev === "munder" || prev === "mover") && base.firstChild && base.firstChild.nodeName === "mo" && base.firstChild.getAttribute("largeop") === "true") {
        isBig = true;
      }
      let tag;
      if (isBig) {
        if (prev === "munder" && sup || prev === "mover" && !sup) tag = "munderover";
        else tag = sup ? "mover" : "munder";
      } else if (prev === "msub" || prev === "msup") {
        tag = "msubsup";
      } else {
        tag = sup ? "msup" : "msub";
      }
      const n = mathEl(tag);
      if (tag === "msubsup" || tag === "munderover") {
        n.appendChild(base.firstChild);
        n.appendChild(base.lastChild);
      } else {
        n.appendChild(base);
      }
      n.appendChild(arg);
      row.appendChild(n);
    }
    function parseMathTable(env) {
      const table = mathEl("mtable");
      let row = mathEl("mtr");
      let guard = 0;
      while (pos < src.length && guard++ < 2e3) {
        skipSpaces();
        if (src.charAt(pos) === "\\" && src.substr(pos, 2) === "\\\\") {
          pos += 2;
          table.appendChild(row);
          row = mathEl("mtr");
          continue;
        }
        if (atEndMarker()) break;
        if (src.charAt(pos) === "\\" && /^\\hline\b/.test(src.slice(pos))) {
          readCommandName();
          continue;
        }
        if (src.charAt(pos) === "&") {
          pos += 1;
          continue;
        }
        const cell = parseExpr();
        const td = mathEl("mtd");
        td.appendChild(cell);
        row.appendChild(td);
        if (pos >= src.length) break;
      }
      if (row.childNodes.length) table.appendChild(row);
      const delims = MATH_MATRIX_ENVS[env];
      if (!delims) return table;
      const wrap = mathEl("mrow");
      const left = mathEl("mo"), right = mathEl("mo");
      left.setAttribute("stretchy", "true");
      right.setAttribute("stretchy", "true");
      left.appendChild(document.createTextNode(delims.charAt(0)));
      right.appendChild(document.createTextNode(delims.charAt(1)));
      wrap.appendChild(left);
      wrap.appendChild(table);
      wrap.appendChild(right);
      return wrap;
    }
    function parseCommand() {
      const name = readCommandName();
      if (name === "\\") {
        const br = mathEl("mspace");
        br.setAttribute("linebreak", "newline");
        return br;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_SPACES, name)) {
        const sp = mathEl("mspace");
        sp.setAttribute("width", MATH_SPACES[name]);
        return sp;
      }
      if (name === "frac" || name === "dfrac" || name === "tfrac") {
        const f = mathEl("mfrac");
        f.appendChild(argGroup());
        f.appendChild(argGroup());
        return f;
      }
      if (name === "sqrt") {
        skipSpaces();
        let root2;
        if (src.charAt(pos) === "[") {
          let close = src.indexOf("]", pos + 1);
          if (close < 0) close = src.length;
          const degSrc = src.slice(pos + 1, close);
          pos = close + 1;
          const deg = mathEl("mrow");
          for (let di = 0; di < degSrc.length; di += 1) {
            const dc = degSrc.charAt(di);
            if (isSpace(dc)) continue;
            deg.appendChild(isDigit(dc) ? mathText("mn", dc) : isLetter(dc) ? mathText("mi", dc) : mathSymbol(dc));
          }
          root2 = mathEl("mroot");
          root2.appendChild(argGroup());
          root2.appendChild(deg);
          return root2;
        }
        root2 = mathEl("msqrt");
        root2.appendChild(argGroup());
        return root2;
      }
      if (name === "left" || name === "right" || name === "big" || name === "Big" || name === "bigl" || name === "bigr" || name === "Bigl" || name === "Bigr" || name === "biggl" || name === "biggr" || name === "Biggl" || name === "Biggr") {
        skipSpaces();
        let d = src.charAt(pos);
        if (d === "\\") {
          const dn = readCommandName();
          d = Object.prototype.hasOwnProperty.call(MATH_OPS, dn) ? MATH_OPS[dn] : dn;
        } else {
          pos += 1;
          if (d === ".") return mathEl("mspace");
        }
        const mo = mathEl("mo");
        mo.setAttribute("stretchy", name === "left" || name === "right" ? "true" : "false");
        mo.appendChild(document.createTextNode(d || ""));
        return mo;
      }
      if (name === "begin") {
        const env = readBraceText();
        if (!Object.prototype.hasOwnProperty.call(MATH_MATRIX_ENVS, env)) {
          return mathText("mtext", "\\begin{" + env + "}");
        }
        if (env === "array") readBraceText();
        const built = parseMathTable(env);
        skipSpaces();
        if (atEndMarker()) {
          readCommandName();
          readBraceText();
        }
        return built;
      }
      if (name === "text" || name === "textrm" || name === "mbox" || name === "operatorname") {
        const tx = mathText("mtext", readBraceText());
        if (name === "operatorname") tx.setAttribute("mathvariant", "normal");
        return tx;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_FONTS, name)) {
        const inner = argGroup();
        inner.setAttribute("mathvariant", MATH_FONTS[name]);
        if (name === "mathbb") {
          (function mapBB(node) {
            for (let i = 0; i < node.childNodes.length; i += 1) {
              const kid = node.childNodes[i];
              const ch = kid.textContent;
              if (kid.nodeType === 3 && ch && ch.length === 1 && MATH_BB[ch]) {
                kid.textContent = MATH_BB[ch];
              } else if (kid.childNodes && kid.childNodes.length) {
                mapBB(kid);
              }
            }
          })(inner);
        }
        return inner;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_ACCENTS, name)) {
        const acc = mathEl("mover");
        acc.setAttribute("accent", "true");
        acc.appendChild(argGroup());
        const am = mathEl("mo");
        am.appendChild(document.createTextNode(MATH_ACCENTS[name]));
        acc.appendChild(am);
        return acc;
      }
      if (name === "underline") {
        const ul = mathEl("munder");
        ul.setAttribute("accentunder", "true");
        ul.appendChild(argGroup());
        const um = mathEl("mo");
        um.appendChild(document.createTextNode("_"));
        ul.appendChild(um);
        return ul;
      }
      if (name === "overbrace" || name === "underbrace" || name === "stackrel" || name === "overset" || name === "underset") {
        const ov = mathEl(name === "underset" || name === "underbrace" ? "munder" : "mover");
        ov.appendChild(argGroup());
        ov.appendChild(argGroup());
        return ov;
      }
      if (name === "displaystyle" || name === "textstyle" || name === "limits" || name === "nolimits" || name === "nonumber" || name === "notag" || name === "label" || name === "tag") {
        if (name === "label" || name === "tag") readBraceText();
        return null;
      }
      if (name === "pmod" || name === "pod") {
        const pm2 = mathEl("mrow");
        pm2.appendChild(mathEl("mspace"));
        pm2.appendChild(mathText("mtext", "("));
        pm2.appendChild(argGroup());
        pm2.appendChild(mathText("mtext", ")"));
        return pm2;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_LETTER_OPS, name)) {
        const big = MATH_LETTER_OPS[name];
        const isWord = !/[\u2200-\u22ff\u2a00-\u2aff]/.test(big);
        const bo = mathEl(isWord ? "mi" : "mo");
        bo.appendChild(document.createTextNode(big));
        if (!isWord) {
          bo.setAttribute("largeop", "true");
          bo.setAttribute("movablelimits", "true");
        } else bo.setAttribute("mathvariant", "normal");
        return bo;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_FUNCS, name)) {
        const fn = mathText("mi", name);
        fn.setAttribute("mathvariant", "normal");
        return fn;
      }
      if (Object.prototype.hasOwnProperty.call(MATH_CHARS, name)) {
        return mathText("mi", MATH_CHARS[name]);
      }
      if (Object.prototype.hasOwnProperty.call(MATH_OPS, name)) {
        return mathSymbol(MATH_OPS[name]);
      }
      return mathText("mtext", "\\" + name);
    }
    function parseAtom() {
      skipSpaces();
      const c = src.charAt(pos);
      if (!c) return null;
      if (c === "{") {
        pos += 1;
        const g = parseExpr();
        if (src.charAt(pos) === "}") pos += 1;
        return g;
      }
      if (c === "\\") return parseCommand();
      if (isDigit(c) || c === "." && isDigit(src.charAt(pos + 1))) {
        const s = pos;
        while (pos < src.length && (isDigit(src[pos]) || src[pos] === ".")) pos += 1;
        return mathText("mn", src.slice(s, pos));
      }
      if (isLetter(c)) {
        pos += 1;
        return mathText("mi", c);
      }
      pos += 1;
      if (c === "~") {
        const nb = mathEl("mspace");
        nb.setAttribute("width", "0.333em");
        return nb;
      }
      return mathSymbol(c);
    }
    function parseExpr() {
      const row = mathEl("mrow");
      let guard = 0;
      while (pos < src.length && guard++ < 4e3) {
        skipSpaces();
        const c = src.charAt(pos);
        if (!c || c === "}" || c === "&") break;
        if (c === "\\" && (src.substr(pos, 2) === "\\\\" || atEndMarker())) break;
        if (c === "^" || c === "_") {
          attachScript(row, c === "^");
          continue;
        }
        const atom = parseAtom();
        if (atom) row.appendChild(atom);
        else if (pos < src.length && src.charAt(pos) === c) pos += 1;
      }
      return row;
    }
    let body = parseExpr();
    if (body.childNodes.length === 1 && body.firstChild && body.firstChild.nodeName === "mrow") {
      body = body.firstChild;
    }
    root.appendChild(body);
    return root;
  }
  function mdCodeBlock(code, lang) {
    const wrap = el$1("div", "md-code-block");
    const head = el$1("div", "md-code-head");
    head.appendChild(el$1("span", "md-code-lang", lang || "text"));
    head.appendChild(copyButton(code));
    wrap.appendChild(head);
    const body = el$1("pre", "md-code-body");
    const inner = el$1("code");
    const res = highlightMachine(inner, code);
    body.appendChild(inner);
    wrap.appendChild(body);
    if (res.note) wrap.appendChild(el$1("div", "note", res.note));
    return wrap;
  }
  function mdTable(header, rows) {
    const wrap = el$1("div", "md-table-wrap");
    const table = el$1("table", "md-table");
    const thead = el$1("thead");
    const hrow = el$1("tr");
    header.forEach((cell) => {
      const th = el$1("th");
      mdInline(th, cell, 0);
      hrow.appendChild(th);
    });
    thead.appendChild(hrow);
    table.appendChild(thead);
    const tbody = el$1("tbody");
    rows.forEach((cells) => {
      const tr = el$1("tr");
      for (let c = 0; c < header.length; c++) {
        const td = el$1("td");
        mdInline(td, cells[c] === void 0 ? "" : cells[c], 0);
        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);
    wrap.appendChild(table);
    return wrap;
  }
  function mdListItem(item) {
    const li = el$1("li", "md-item");
    mdInlineLines(li, item.text);
    return li;
  }
  function renderMarkdown(text) {
    const frag = document.createDocumentFragment();
    const lines = String(text === void 0 || text === null ? "" : text).replace(/\r\n?/g, "\n").split("\n");
    let i = 0;
    while (i < lines.length) {
      const line = lines[i];
      const trimmed = line.trim();
      if (!trimmed) {
        i++;
        continue;
      }
      const fence = mdFence(line);
      if (fence) {
        const code = [];
        i++;
        const close = new RegExp("^\\s{0,3}" + (fence.mark === "`" ? "`" : "~") + "{3,}\\s*$");
        while (i < lines.length && !close.test(lines[i])) {
          code.push(lines[i]);
          i++;
        }
        if (i < lines.length) i++;
        frag.appendChild(mdCodeBlock(code.join("\n"), fence.lang));
        continue;
      }
      const h = MD_HEADING.exec(trimmed);
      if (h) {
        const level = h[1].length;
        const heading = el$1("h" + level, "md-h md-h" + level);
        mdInlineLines(heading, h[2]);
        frag.appendChild(heading);
        i++;
        continue;
      }
      if (MD_HR.test(trimmed)) {
        frag.appendChild(el$1("hr", "md-hr"));
        i++;
        continue;
      }
      if (trimmed.indexOf("|") >= 0 && i + 1 < lines.length && mdSeparatorRow(lines[i + 1])) {
        const header = mdSplitRow(trimmed);
        if (header.length > 1) {
          const rows = [];
          i += 2;
          while (i < lines.length && lines[i].trim() && lines[i].indexOf("|") >= 0) {
            rows.push(mdSplitRow(lines[i]));
            i++;
          }
          frag.appendChild(mdTable(header, rows));
          continue;
        }
      }
      if (/^\s{0,3}>/.test(line)) {
        const quote = [];
        while (i < lines.length && /^\s{0,3}>/.test(lines[i])) {
          quote.push(lines[i].replace(/^\s{0,3}>\s?/, ""));
          i++;
        }
        const bq = el$1("blockquote", "md-quote");
        bq.appendChild(renderMarkdown(quote.join("\n")));
        frag.appendChild(bq);
        continue;
      }
      if (mdListMarker(line)) {
        const items = [];
        while (i < lines.length) {
          const mk = mdListMarker(lines[i]);
          if (mk) {
            items.push(mk);
            i++;
            continue;
          }
          if (items.length && lines[i].trim() && i + 1 <= lines.length && !mdBlockStart(lines[i], lines[i + 1])) {
            items[items.length - 1].text += "\n" + lines[i].trim();
            i++;
            continue;
          }
          break;
        }
        const baseIndent = items[0].indent;
        let list = null;
        let listOrdered = false;
        let lastLi = null;
        let sub = null;
        items.forEach((it) => {
          if (!list || it.indent <= baseIndent && it.ordered !== listOrdered) {
            list = el$1(it.ordered ? "ol" : "ul", "md-list");
            listOrdered = it.ordered;
            lastLi = null;
            sub = null;
            frag.appendChild(list);
          }
          if (it.indent > baseIndent && lastLi) {
            if (!sub) {
              sub = el$1(it.ordered ? "ol" : "ul", "md-list md-sub");
              lastLi.appendChild(sub);
            }
            sub.appendChild(mdListItem(it));
            return;
          }
          sub = null;
          lastLi = mdListItem(it);
          list.appendChild(lastLi);
        });
        continue;
      }
      const buf = [];
      while (i < lines.length && !mdBlockStart(lines[i], lines[i + 1])) {
        buf.push(lines[i]);
        i++;
      }
      if (!buf.length) {
        buf.push(lines[i]);
        i++;
      }
      const para = el$1("p", "md-p");
      mdInlineLines(para, buf.join("\n"));
      frag.appendChild(para);
    }
    return frag;
  }
  const MD_INLINE = /(`+)([^`]*?)\1|\[([^\]]*)\]\(([^)\s]*)\)|\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~|\*([^*\n]+)\*|_([^_\n]+)_|\$\$([\s\S]+?)\$\$|\$([^$\n]+?)\$/;
  function mdSafeURL(url) {
    const s = String(url === void 0 || url === null ? "" : url).trim();
    if (!s) return "";
    if (/^(https?:|mailto:|#|\/|\.\/|\.\.\/)/i.test(s)) return s;
    if (/^[a-z][a-z0-9+.-]*:/i.test(s)) return "";
    return s;
  }
  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== void 0 && text !== null) node.textContent = text;
    return node;
  }
  function mdInline(parent, text, depth = 0) {
    if (depth > 6) {
      parent.appendChild(document.createTextNode(String(text || "")));
      return;
    }
    let rest = String(text === void 0 || text === null ? "" : text);
    let guard = 0;
    while (rest && guard++ < 800) {
      const m = MD_INLINE.exec(rest);
      if (!m) break;
      if (m.index > 0) parent.appendChild(document.createTextNode(rest.slice(0, m.index)));
      rest = rest.slice(m.index + m[0].length);
      let node;
      if (m[1] !== void 0) {
        node = el("code", "md-inline-code", m[2]);
      } else if (m[3] !== void 0) {
        node = el("a", "md-link");
        const href = mdSafeURL(m[4]);
        if (href) {
          node.setAttribute("href", href);
          node.setAttribute("target", "_blank");
          node.setAttribute("rel", "noopener noreferrer");
        } else {
          node.title = "链接协议不受支持，只显示文字";
        }
        mdInline(node, m[3], depth + 1);
      } else if (m[10] !== void 0 || m[11] !== void 0) {
        try {
          node = mdMathML(m[10] !== void 0 ? m[10] : m[11], m[10] !== void 0);
        } catch {
          node = el("span");
          node.appendChild(document.createTextNode(m[0]));
        }
      } else if (m[5] !== void 0 || m[6] !== void 0) {
        node = el("strong");
        mdInline(node, m[5] !== void 0 ? m[5] : m[6], depth + 1);
      } else if (m[7] !== void 0) {
        node = el("del");
        mdInline(node, m[7], depth + 1);
      } else {
        node = el("em");
        mdInline(node, m[8] !== void 0 ? m[8] : m[9], depth + 1);
      }
      parent.appendChild(node);
    }
    if (rest) parent.appendChild(document.createTextNode(rest));
  }
  function renderInlineMarkdown(text) {
    const frag = document.createDocumentFragment();
    mdInline(frag, text, 0);
    return frag;
  }
  function mdInlineLines(parent, text) {
    String(text === void 0 || text === null ? "" : text).split("\n").forEach((part, i) => {
      if (i) parent.appendChild(el("br", "md-br"));
      mdInline(parent, part, 0);
    });
  }
  const _sfc_main$j = /* @__PURE__ */ defineComponent({
    __name: "InlineMD",
    props: {
      tag: {},
      text: {}
    },
    setup(__props) {
      const props = __props;
      const host = /* @__PURE__ */ ref(null);
      function render() {
        const el2 = host.value;
        if (!el2) return;
        el2.textContent = "";
        el2.appendChild(renderInlineMarkdown(props.text));
      }
      onMounted(render);
      watch(() => props.text, render);
      return (_ctx, _cache) => {
        return openBlock(), createBlock(resolveDynamicComponent(props.tag || "span"), {
          ref_key: "host",
          ref: host
        }, null, 512);
      };
    }
  });
  const _hoisted_1$e = {
    id: "sidebar-col",
    class: "sidebar-col"
  };
  const _hoisted_2$d = { class: "side-head" };
  const _hoisted_3$a = ["title"];
  const _hoisted_4$7 = { class: "side-search" };
  const _hoisted_5$6 = { class: "side-list-wrap" };
  const _hoisted_6$6 = ["data-project"];
  const _hoisted_7$5 = ["title"];
  const _hoisted_8$4 = { class: "row-body" };
  const _hoisted_9$4 = {
    key: 0,
    class: "proj-prefix"
  };
  const _hoisted_10$3 = { class: "proj-title" };
  const _hoisted_11$3 = { class: "row-meta" };
  const _hoisted_12$3 = {
    key: 0,
    class: "proj-note",
    title: "这个输出根本身就是一个工程（work/、progress.json 等直接挂在它下面），是引入多项目布局之前的形态；新布局是「输出根/书名/」。"
  };
  const _hoisted_13$3 = {
    key: 1,
    class: "dot live"
  };
  const _hoisted_14$3 = {
    key: 0,
    class: "proj-progress",
    title: "progress.json 里各阶段的当前状态"
  };
  const _hoisted_15$3 = ["data-project", "data-stage"];
  const _hoisted_16$3 = { class: "stage-row" };
  const _hoisted_17$3 = { class: "row-body" };
  const _hoisted_18$3 = { class: "row-meta" };
  const _hoisted_19$3 = ["title"];
  const _hoisted_20$3 = {
    key: 0,
    class: "session-blocks"
  };
  const _hoisted_21$3 = ["data-id", "title", "onClick"];
  const _hoisted_22$3 = { key: 1 };
  const _hoisted_23$3 = ["data-id", "title", "onClick"];
  const _hoisted_24$3 = { class: "row-slot" };
  const _hoisted_25$2 = {
    key: 0,
    class: "row-chip usage-chip"
  };
  const _hoisted_26$2 = ["title"];
  const _hoisted_27$2 = { class: "row-time" };
  const _hoisted_28$2 = { class: "row-actions" };
  const _hoisted_29$1 = ["onClick"];
  const _hoisted_30$1 = ["onClick"];
  const _hoisted_31$1 = {
    key: 0,
    class: "session-blocks"
  };
  const _hoisted_32$1 = ["data-id", "title", "onClick"];
  const _hoisted_33$1 = ["data-id", "title", "onClick"];
  const _hoisted_34$1 = { class: "row-slot" };
  const _hoisted_35$1 = {
    key: 0,
    class: "row-chip usage-chip"
  };
  const _hoisted_36$1 = ["title"];
  const _hoisted_37$1 = { class: "row-time" };
  const _hoisted_38$1 = { class: "row-actions" };
  const _hoisted_39$1 = ["onClick"];
  const _hoisted_40$1 = ["onClick"];
  const _hoisted_41$1 = {
    key: 0,
    class: "empty"
  };
  const _hoisted_42$1 = { class: "side-status" };
  const _hoisted_43$1 = {
    id: "root-path",
    class: "root-path",
    title: "扫描根目录"
  };
  const _hoisted_44$1 = {
    id: "side-foot",
    class: "side-foot"
  };
  const OVERFLOW_LIMIT = 8;
  const _sfc_main$i = /* @__PURE__ */ defineComponent({
    __name: "Sidebar",
    setup(__props) {
      const listEl = /* @__PURE__ */ ref(null);
      const blockView = /* @__PURE__ */ ref(storeGet("side.blockView") === "1");
      function toggleBlockView() {
        blockView.value = !blockView.value;
        storeSet("side.blockView", blockView.value ? "1" : "0");
      }
      const groups = computed(() => {
        return buildGroups().map((g) => {
          const stages0 = g.items.length ? g.items[0].projectStages : null;
          const stageKeys = Object.keys(g.stages).sort((a, b) => {
            const d = stageRank(a) - stageRank(b);
            return d !== 0 ? d : a < b ? -1 : 1;
          });
          const multi = stageKeys.length > 1;
          const stages = stageKeys.map((stg) => {
            const sg = g.stages[stg];
            const items = sg.items.slice().sort((x, y) => {
              if (x.imageOrder && y.imageOrder && x.imageOrder !== y.imageOrder) return x.imageOrder - y.imageOrder;
              return y.mtime > x.mtime ? 1 : -1;
            });
            const okey = groupKey("overflow", g.name + "/" + stg);
            const needOverflow = items.length > OVERFLOW_LIMIT;
            const openAll = !needOverflow || isOverflowOpen(okey) || !!state.filter;
            const shown = openAll ? items : items.slice(0, OVERFLOW_LIMIT);
            return {
              stage: stg,
              title: stageTitleOf(stg),
              items,
              live: sg.live,
              status: stageStatusText(stages0, stg, sg.live),
              overflowKey: okey,
              needOverflow,
              shown,
              hiddenCount: items.length - shown.length
            };
          });
          return {
            name: g.name,
            prefix: g.prefix,
            title: g.title,
            legacy: g.legacy,
            items: g.items,
            live: g.live,
            matched: g.matched,
            progress: projectProgressLine(stages0),
            stages,
            multi
          };
        });
      });
      const shownCount = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0));
      const emptyText = computed(() => state.sessions.length ? "没有匹配的会话" : "没有找到 *.jsonl 会话转录");
      const footText = computed(() => {
        const live = state.sessions.filter((s) => s.live).length;
        return groups.value.length + " 个项目 · " + state.sessions.length + " 个会话" + (live ? " · " + live + " 个活跃" : "") + (state.filter ? " · 匹配 " + shownCount.value : "");
      });
      const totals = computed(() => {
        const tot = { requests: 0, prompt: 0, cached: 0, completion: 0, cost: 0, costCurrency: "", unpriced: 0 };
        state.sessions.forEach((s) => {
          const st = s.stats;
          if (!st || !st.requests) return;
          tot.requests += st.requests;
          tot.prompt += st.promptTokens;
          tot.cached += st.cachedTokens;
          tot.completion += st.completionTokens;
          if (s.cost) {
            tot.cost += Number(s.cost.total) || 0;
            tot.costCurrency = s.cost.currency || tot.costCurrency;
          } else {
            tot.unpriced += st.requests;
          }
        });
        return tot;
      });
      const totalsText = computed(() => {
        const tot = totals.value;
        if (!tot.requests) return "";
        let money = "";
        if (tot.cost > 0 || tot.costCurrency) {
          money = " · 费用 " + (tot.unpriced ? "≥" : "") + tot.costCurrency + tot.cost.toFixed(2);
        }
        return "合计 " + tot.requests + " 次请求 · 输入 " + fmtTokens(tot.prompt) + " / 输出 " + fmtTokens(tot.completion) + " tokens" + (tot.prompt ? " · 缓存命中 " + (tot.cached * 100 / tot.prompt).toFixed(0) + "%" : "") + money;
      });
      const vCollapse = {
        mounted(el2, binding) {
          const { key, want, frozen } = binding.value;
          el2.open = !!want;
          el2.dataset.open = want ? "1" : "0";
          el2.addEventListener("toggle", () => {
            const now = el2.open ? "1" : "0";
            if (el2.dataset.open === now) return;
            el2.dataset.open = now;
            if (frozen) return;
            setCollapsed(key, !el2.open);
          });
        },
        updated(el2, binding) {
          const { want } = binding.value;
          const flag = want ? "1" : "0";
          if (el2.dataset.open === flag) return;
          el2.dataset.open = flag;
          el2.open = !!want;
        }
      };
      function projWantOpen(g) {
        const cur = state.current;
        const curProj = cur ? cur.project || projectOf(cur.id) : "";
        return groupWantOpen(groupKey("proj", g.name), !!curProj && curProj === g.name);
      }
      function stageWantOpen(g, sv) {
        const cur = state.current;
        const curProj = cur ? cur.project || projectOf(cur.id) : "";
        const curStage = cur ? cur.stage || "session" : "";
        return groupWantOpen(
          groupKey("stage", g.name + "/" + sv.stage),
          !!curProj && curProj === g.name && curStage === sv.stage
        );
      }
      function rowTitle(s) {
        return sessionTip(s);
      }
      function onRowClick(s) {
        selectSession(s.id);
      }
      function onInfoClick(ev, s) {
        ev.stopPropagation();
        selectSession(s.id);
        openDetails();
      }
      function onMoreClick(okey) {
        setOverflowOpen(okey);
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("aside", _hoisted_1$e, [
          createBaseVNode("div", _hoisted_2$d, [
            _cache[2] || (_cache[2] = createBaseVNode("span", { class: "side-title" }, "工作区", -1)),
            createBaseVNode("button", {
              id: "side-view-toggle",
              class: normalizeClass(["icon-btn", { on: blockView.value }]),
              type: "button",
              title: blockView.value ? "切回列表视图" : "切到方块视图（会话多时好扫；悬浮看说明）",
              onClick: toggleBlockView
            }, toDisplayString(blockView.value ? "☰" : "▦"), 11, _hoisted_3$a),
            createBaseVNode("button", {
              id: "refresh",
              class: "icon-btn",
              type: "button",
              title: "重新扫描会话",
              onClick: _cache[0] || (_cache[0] = //@ts-ignore
              (...args) => unref(refreshIndex) && unref(refreshIndex)(...args))
            }, "⟳")
          ]),
          createBaseVNode("div", _hoisted_4$7, [
            withDirectives(createBaseVNode("input", {
              id: "search",
              "onUpdate:modelValue": _cache[1] || (_cache[1] = ($event) => unref(state).filter = $event),
              type: "search",
              placeholder: "过滤：会话名 / 阶段 / 项目…",
              autocomplete: "off"
            }, null, 512), [
              [vModelText, unref(state).filter]
            ])
          ]),
          createBaseVNode("div", _hoisted_5$6, [
            createBaseVNode("div", {
              id: "session-list",
              ref_key: "listEl",
              ref: listEl,
              class: "session-list",
              role: "tree",
              "aria-label": "会话列表"
            }, [
              (openBlock(true), createElementBlock(Fragment, null, renderList(groups.value, (g) => {
                return withDirectives((openBlock(), createElementBlock("details", {
                  key: g.name,
                  class: "proj-group",
                  "data-project": g.name
                }, [
                  createBaseVNode("summary", {
                    class: "proj-row",
                    title: g.name
                  }, [
                    _cache[3] || (_cache[3] = createStaticVNode('<span class="row-slot row-folder" data-v-16630052><svg class="folder closed" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" data-v-16630052><path d="M1.5 3.5h4l1.5 2h7.5v7a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-9Z" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" data-v-16630052></path></svg><svg class="folder open" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" data-v-16630052><path d="M14.5 8V5.5a1 1 0 0 0-1-1H7.2L5.7 3.5H2.5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h2.2" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" stroke-linecap="round" data-v-16630052></path><path d="M4.9 14.5 6.7 7.5h7.7l-1.8 7Z" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" data-v-16630052></path></svg></span>', 1)),
                    createBaseVNode("span", _hoisted_8$4, [
                      g.prefix ? (openBlock(), createElementBlock("span", _hoisted_9$4, toDisplayString(g.prefix), 1)) : createCommentVNode("", true),
                      createBaseVNode("span", _hoisted_10$3, toDisplayString(g.title), 1)
                    ]),
                    createBaseVNode("span", _hoisted_11$3, toDisplayString(g.items.length) + " 个会话", 1),
                    g.legacy ? (openBlock(), createElementBlock("span", _hoisted_12$3, "旧版单项目")) : createCommentVNode("", true),
                    g.live ? (openBlock(), createElementBlock("span", _hoisted_13$3)) : createCommentVNode("", true)
                  ], 8, _hoisted_7$5),
                  createBaseVNode("div", null, [
                    g.progress ? (openBlock(), createElementBlock("div", _hoisted_14$3, toDisplayString(g.progress), 1)) : createCommentVNode("", true),
                    (openBlock(true), createElementBlock(Fragment, null, renderList(g.stages, (sv) => {
                      return openBlock(), createElementBlock(Fragment, {
                        key: sv.stage
                      }, [
                        g.multi ? withDirectives((openBlock(), createElementBlock("details", {
                          key: 0,
                          class: "stage-group",
                          "data-project": g.name,
                          "data-stage": sv.stage
                        }, [
                          createBaseVNode("summary", _hoisted_16$3, [
                            _cache[4] || (_cache[4] = createBaseVNode("span", { class: "row-slot" }, [
                              createBaseVNode("span", { class: "row-caret" })
                            ], -1)),
                            createBaseVNode("span", _hoisted_17$3, toDisplayString(sv.title), 1),
                            createBaseVNode("span", _hoisted_18$3, toDisplayString(sv.items.length) + " 个会话", 1),
                            sv.status ? (openBlock(), createElementBlock("span", {
                              key: 0,
                              class: normalizeClass(["stage-progress", sv.status.state]),
                              title: sv.status.title
                            }, toDisplayString(sv.status.text), 11, _hoisted_19$3)) : createCommentVNode("", true)
                          ]),
                          blockView.value ? (openBlock(), createElementBlock("div", _hoisted_20$3, [
                            (openBlock(true), createElementBlock(Fragment, null, renderList(sv.items, (s) => {
                              return openBlock(), createElementBlock("button", {
                                key: s.id,
                                class: normalizeClass(["session-block", { active: unref(state).current && unref(state).current.id === s.id, live: s.live, err: !s.live && s.endState === "error", pend: !s.live && !s.endState }]),
                                type: "button",
                                role: "treeitem",
                                "data-id": s.id,
                                title: rowTitle(s),
                                onClick: ($event) => onRowClick(s)
                              }, toDisplayString(s.imageOrder || s.chapterOrder || ""), 11, _hoisted_21$3);
                            }), 128))
                          ])) : (openBlock(), createElementBlock("div", _hoisted_22$3, [
                            (openBlock(true), createElementBlock(Fragment, null, renderList(sv.shown, (s) => {
                              return openBlock(), createElementBlock("button", {
                                key: s.id,
                                class: normalizeClass(["session-row sub2", { active: unref(state).current && unref(state).current.id === s.id }]),
                                type: "button",
                                role: "treeitem",
                                "data-id": s.id,
                                title: rowTitle(s),
                                onClick: ($event) => onRowClick(s)
                              }, [
                                createBaseVNode("span", _hoisted_24$3, [
                                  createBaseVNode("span", {
                                    class: normalizeClass(["dot", { live: s.live }])
                                  }, null, 2)
                                ]),
                                createVNode(_sfc_main$j, {
                                  tag: "span",
                                  class: "row-title",
                                  text: unref(sessionTitleOf)(s)
                                }, null, 8, ["text"]),
                                unref(usageChipText)(s) ? (openBlock(), createElementBlock("span", _hoisted_25$2, toDisplayString(unref(usageChipText)(s)), 1)) : createCommentVNode("", true),
                                s.imageName ? (openBlock(), createElementBlock("span", {
                                  key: 1,
                                  class: "row-chip image-chip",
                                  title: unref(imageTipText)(s)
                                }, toDisplayString(unref(imageChipText)(s)), 9, _hoisted_26$2)) : createCommentVNode("", true),
                                createBaseVNode("span", _hoisted_27$2, toDisplayString(unref(relTime)(s.mtime)), 1),
                                createBaseVNode("span", _hoisted_28$2, [
                                  createBaseVNode("button", {
                                    class: "icon-btn",
                                    type: "button",
                                    title: "打开详情面板（元信息 / 指标）",
                                    onClick: withModifiers(($event) => onInfoClick($event, s), ["stop"])
                                  }, "ⓘ", 8, _hoisted_29$1)
                                ])
                              ], 10, _hoisted_23$3);
                            }), 128)),
                            sv.needOverflow ? (openBlock(), createElementBlock("button", {
                              key: 0,
                              class: "session-overflow",
                              type: "button",
                              onClick: ($event) => onMoreClick(sv.overflowKey)
                            }, " 更多会话（还有 " + toDisplayString(sv.hiddenCount) + " 个） ", 9, _hoisted_30$1)) : createCommentVNode("", true)
                          ]))
                        ], 8, _hoisted_15$3)), [
                          [vCollapse, { key: "stage:" + g.name + "/" + sv.stage, want: stageWantOpen(g, sv), frozen: false }]
                        ]) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
                          blockView.value ? (openBlock(), createElementBlock("div", _hoisted_31$1, [
                            (openBlock(true), createElementBlock(Fragment, null, renderList(sv.items, (s) => {
                              return openBlock(), createElementBlock("button", {
                                key: s.id,
                                class: normalizeClass(["session-block", { active: unref(state).current && unref(state).current.id === s.id, live: s.live, err: !s.live && s.endState === "error", pend: !s.live && !s.endState }]),
                                type: "button",
                                role: "treeitem",
                                "data-id": s.id,
                                title: rowTitle(s),
                                onClick: ($event) => onRowClick(s)
                              }, toDisplayString(s.imageOrder || s.chapterOrder || ""), 11, _hoisted_32$1);
                            }), 128))
                          ])) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
                            (openBlock(true), createElementBlock(Fragment, null, renderList(sv.shown, (s) => {
                              return openBlock(), createElementBlock("button", {
                                key: s.id,
                                class: normalizeClass(["session-row sub1", { active: unref(state).current && unref(state).current.id === s.id }]),
                                type: "button",
                                role: "treeitem",
                                "data-id": s.id,
                                title: rowTitle(s),
                                onClick: ($event) => onRowClick(s)
                              }, [
                                createBaseVNode("span", _hoisted_34$1, [
                                  createBaseVNode("span", {
                                    class: normalizeClass(["dot", { live: s.live }])
                                  }, null, 2)
                                ]),
                                createVNode(_sfc_main$j, {
                                  tag: "span",
                                  class: "row-title",
                                  text: unref(sessionTitleOf)(s)
                                }, null, 8, ["text"]),
                                unref(usageChipText)(s) ? (openBlock(), createElementBlock("span", _hoisted_35$1, toDisplayString(unref(usageChipText)(s)), 1)) : createCommentVNode("", true),
                                s.imageName ? (openBlock(), createElementBlock("span", {
                                  key: 1,
                                  class: "row-chip image-chip",
                                  title: unref(imageTipText)(s)
                                }, toDisplayString(unref(imageChipText)(s)), 9, _hoisted_36$1)) : createCommentVNode("", true),
                                createBaseVNode("span", _hoisted_37$1, toDisplayString(unref(relTime)(s.mtime)), 1),
                                createBaseVNode("span", _hoisted_38$1, [
                                  createBaseVNode("button", {
                                    class: "icon-btn",
                                    type: "button",
                                    title: "打开详情面板（元信息 / 指标）",
                                    onClick: withModifiers(($event) => onInfoClick($event, s), ["stop"])
                                  }, "ⓘ", 8, _hoisted_39$1)
                                ])
                              ], 10, _hoisted_33$1);
                            }), 128)),
                            sv.needOverflow ? (openBlock(), createElementBlock("button", {
                              key: 0,
                              class: "session-overflow",
                              type: "button",
                              onClick: ($event) => onMoreClick(sv.overflowKey)
                            }, " 更多会话（还有 " + toDisplayString(sv.hiddenCount) + " 个） ", 9, _hoisted_40$1)) : createCommentVNode("", true)
                          ], 64))
                        ], 64))
                      ], 64);
                    }), 128))
                  ])
                ], 8, _hoisted_6$6)), [
                  [vCollapse, { key: "proj:" + g.name, want: g.matched ? true : projWantOpen(g), frozen: g.matched }]
                ]);
              }), 128)),
              !shownCount.value ? (openBlock(), createElementBlock("div", _hoisted_41$1, toDisplayString(emptyText.value), 1)) : createCommentVNode("", true)
            ], 512),
            _cache[5] || (_cache[5] = createBaseVNode("div", {
              class: "list-fade",
              "aria-hidden": "true"
            }, null, -1))
          ]),
          createBaseVNode("div", {
            id: "side-totals",
            class: normalizeClass(["side-totals", { hidden: !totalsText.value }])
          }, toDisplayString(totalsText.value), 3),
          createBaseVNode("div", _hoisted_42$1, [
            createBaseVNode("div", _hoisted_43$1, toDisplayString(unref(state).root || "—"), 1),
            createBaseVNode("div", _hoisted_44$1, toDisplayString(footText.value), 1)
          ])
        ]);
      };
    }
  });
  const _export_sfc = (sfc, props) => {
    const target = sfc.__vccOpts || sfc;
    for (const [key, val] of props) {
      target[key] = val;
    }
    return target;
  };
  const Sidebar = /* @__PURE__ */ _export_sfc(_sfc_main$i, [["__scopeId", "data-v-16630052"]]);
  function refBaseName(ref2) {
    const s = String(ref2 || "").split("?")[0];
    const i = Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\"));
    return i >= 0 ? s.slice(i + 1) : s;
  }
  function shortFileName(name) {
    const s = String(name || "");
    const dot = s.lastIndexOf(".");
    const stem = dot > 0 ? s.slice(0, dot) : s;
    const ext = dot > 0 ? s.slice(dot) : "";
    if (/^[0-9a-f]{32,}$/i.test(stem)) return stem.slice(0, 8) + ext;
    return s;
  }
  function originalFigureSize(text) {
    const m = /ORIGINAL\s+FIGURE\s+SIZE\s*:\s*([0-9.]+\s*mm\s*[x×]\s*[0-9.]+\s*mm)/i.exec(String(text || ""));
    return m ? m[1].replace(/\s+/g, " ") : "";
  }
  function imageTurnSummary(line) {
    const imgs = line && line.images || [];
    const bits = [imgs.length + " 张图片"];
    const size = originalFigureSize(line && line.text);
    if (size) bits.push(size);
    const names = imgs.map((r) => shortFileName(refBaseName(r)));
    if (names.length) {
      bits.push(names.slice(0, 2).join("、") + (names.length > 2 ? " 等 " + names.length + " 个文件" : ""));
    }
    return bits.join(" · ");
  }
  const IMAGE_HANDLE_RE = /^Tool image output\b/i;
  const IMAGE_CALL_RE = /\(call\s+([A-Za-z0-9_.:-]+)\)/;
  const IMAGE_FROM_RE = /\bfrom\s+([A-Za-z0-9_.:-]+)/i;
  const IMAGE_TOOLS = { view_image: 1, view_pdf: 1 };
  const IMAGE_RESULT_RE = /^(?:Image\s+\S+|PDF page\s+\S+)[^\n]*\battached\b/im;
  const IMAGE_WIRE_TITLE = "user 消息承载图片（tool 消息的 content 只能文本，OpenAI 兼容 schema 限制） · text + image_url(data:image/jpeg;base64,…)";
  function imageAttributions() {
    const sig = (state.current ? state.current.id : "") + ":" + state.lines.length;
    if (state.imgAttr && state.imgAttr.sig === sig) return state.imgAttr.map;
    const calls = [];
    const idxById = {};
    state.lines.forEach((line) => {
      if (!line || line.bad || line.t && line.t !== "msg") return;
      if (line.role === "assistant") {
        (line.tool_calls || []).forEach((c) => {
          (idxById[c.id || ""] = idxById[c.id || ""] || []).push(calls.length);
          calls.push({
            id: c.id || "",
            name: (c.function || {}).name || "",
            lineN: line.n,
            receipt: null,
            claimed: false,
            qpos: -1
          });
        });
        return;
      }
      if (line.role === "tool") {
        const idxs = idxById[line.tool_call_id || ""] || [];
        for (let k = 0; k < idxs.length; k++) {
          if (calls[idxs[k]].receipt === null) {
            calls[idxs[k]].receipt = String(line.text || "");
            break;
          }
        }
      }
    });
    const queue2 = [];
    const candRound = {};
    const roundLast = {};
    calls.forEach((c) => {
      const img = c.receipt === null ? !!IMAGE_TOOLS[c.name] : IMAGE_RESULT_RE.test(c.receipt.trim());
      if (img) {
        c.qpos = queue2.length;
        queue2.push(c);
        candRound[c.id] = c.lineN;
        roundLast[c.lineN] = c.id;
      }
    });
    let firstTask = 0;
    state.lines.forEach((l) => {
      if (!l || l.bad || l.t && l.t !== "msg") return;
      if (l.role === "user" && !(l.images && l.images.length) && !firstTask) firstTask = l.n;
    });
    const map = {};
    let qi = 0;
    const nextUnclaimed = (lineN) => {
      while (qi < queue2.length && queue2[qi].claimed) qi++;
      if (qi >= queue2.length || queue2[qi].lineN >= lineN) return null;
      return queue2[qi];
    };
    const claim = (entry, pick, how) => {
      pick.claimed = true;
      entry.kind = "call";
      entry.how = how;
      entry.callId = pick.id;
      if (!entry.name) entry.name = pick.name;
    };
    state.lines.forEach((line) => {
      if (!line || line.bad || line.t && line.t !== "msg") return;
      if (line.role !== "user" || !(line.images && line.images.length)) return;
      const text = String(line.text || "").trim();
      const entry = { kind: "none", how: "", callId: "", name: "", lineN: line.n, taskLineN: firstTask };
      if (text && !IMAGE_HANDLE_RE.test(text)) {
        entry.kind = "task";
        entry.how = "task";
        entry.taskLineN = line.n;
        map[line.n] = entry;
        return;
      }
      let pick = null;
      if (IMAGE_HANDLE_RE.test(text)) {
        const idm = IMAGE_CALL_RE.exec(text);
        const frm = IMAGE_FROM_RE.exec(text);
        entry.name = frm ? frm[1] : "";
        if (idm) {
          const cands = idxById[idm[1]] || [];
          for (let k = cands.length - 1; k >= 0; k--) {
            const ex = calls[cands[k]];
            if (ex.qpos >= 0 && !ex.claimed && ex.lineN < line.n) {
              pick = ex;
              break;
            }
          }
          if (!pick && idxById[idm[1]]) {
            pick = { id: idm[1], name: entry.name || "", lineN: 0, receipt: null, claimed: false, qpos: -1 };
          }
          if (pick) claim(entry, pick, "call-id");
        }
        if (!pick && entry.name) {
          for (let j = qi; j < queue2.length; j++) {
            if (queue2[j].lineN >= line.n) break;
            if (!queue2[j].claimed && queue2[j].name === entry.name) {
              pick = queue2[j];
              claim(entry, pick, "tool-name");
              break;
            }
          }
        }
      }
      if (!pick) {
        pick = nextUnclaimed(line.n);
        if (pick) claim(entry, pick, "order");
      }
      if (!pick) {
        if (!IMAGE_HANDLE_RE.test(text)) {
          entry.kind = "task";
          entry.how = "task";
          entry.taskLineN = text ? line.n : firstTask || line.n;
        }
      }
      map[line.n] = entry;
    });
    state.imgAttr = { sig, map, candRound, roundLast };
    return state.imgAttr.map;
  }
  function attributionText(attr) {
    if (!attr) return "";
    switch (attr.how) {
      case "call-id":
        return "归属：call " + attr.callId + (attr.name ? "（" + attr.name + "）" : "");
      case "tool-name":
        return "归属：call " + attr.callId + "（按工具名 " + attr.name + " 匹配）";
      case "order":
        return "归属：由顺序推断（本轮的 call " + attr.callId + "）";
      case "task":
        return "归属：本会话任务（这一段是投喂给任务的原图）";
      default:
        return "归属：未识别";
    }
  }
  function sessionDir(id) {
    const i = String(id || "").lastIndexOf("/");
    return i < 0 ? "" : String(id).slice(0, i);
  }
  function mediaURL(ref2) {
    const tail = String(ref2 || "").replace(/^file:\/\//, "");
    const rel = joinPath(sessionDir(state.current ? state.current.id : ""), tail);
    return "/media/" + rel;
  }
  function joinPath(...args) {
    const parts = [];
    for (const a of args) {
      const p2 = String(a || "").replace(/^\/+|\/+$/g, "");
      if (p2) parts.push(p2);
    }
    return parts.join("/");
  }
  function toolFamily(name) {
    const n = String(name).toLowerCase();
    if (n === "bash" || n === "compile" || n === "python") return "shell";
    if (n === "submit") return "submit";
    if (n.indexOf("write") === 0 || n.indexOf("edit") === 0) return "write";
    if (n.indexOf("grep") === 0 || n.indexOf("doc_search") === 0 || n.indexOf("list_") === 0 || n.indexOf("search") >= 0) return "search";
    if (n.indexOf("read") === 0 || n.indexOf("view_") === 0 || n.indexOf("image_context") === 0) return "view";
    return "other";
  }
  function toolSummary(name, argsText) {
    let obj = null;
    try {
      obj = JSON.parse(String(argsText || "{}"));
    } catch {
      obj = null;
    }
    if (!obj || typeof obj !== "object") return "";
    const n = String(name || "").toLowerCase();
    const pick = (v2) => {
      if (typeof v2 === "string") return v2;
      if (v2 === void 0 || v2 === null) return "";
      try {
        return JSON.stringify(v2);
      } catch {
        return "";
      }
    };
    let v = "";
    if (n === "bash" || n === "python") v = pick(obj.command || obj.code || obj.script);
    else if (n === "compile") v = pick(obj.path);
    else if (n.indexOf("grep") === 0 || n.indexOf("search") >= 0 || n === "doc_search") v = pick(obj.pattern || obj.query);
    else if (n.indexOf("write") === 0 || n.indexOf("read") === 0 || n === "view_pdf" || n === "view_image") v = pick(obj.path);
    else if (n === "submit") v = pick(obj.path || obj.status);
    if (!v) {
      const keys = Object.keys(obj);
      for (let i = 0; i < keys.length; i++) {
        const cand = pick(obj[keys[i]]);
        if (cand) {
          v = cand;
          break;
        }
      }
    }
    v = String(v).split("\n")[0].replace(/\s+/g, " ").trim();
    return v.length > 90 ? v.slice(0, 90) + "…" : v;
  }
  function toolPromptLine(name, argsText) {
    let obj = null;
    try {
      obj = JSON.parse(String(argsText || "{}"));
    } catch {
      obj = null;
    }
    if (!obj || typeof obj !== "object") return "";
    const n = String(name).toLowerCase();
    let v = "";
    if (n === "bash" || n === "python") v = obj.command || obj.code || "";
    else if (n.indexOf("grep") === 0 || n === "doc_search" || n.indexOf("search") >= 0) {
      v = [obj.pattern || obj.query || "", obj.path || ""].filter(Boolean).join("  ");
    } else if (n === "compile" || n.indexOf("write") === 0 || n.indexOf("edit") === 0 || n.indexOf("read") === 0 || n === "view_pdf" || n === "view_image") {
      v = obj.path || "";
    }
    const out = String(v).split("\n")[0].trim();
    return out.length > 160 ? out.slice(0, 160) + "…" : out;
  }
  function classifyResult(text) {
    const s = String(text || "");
    if (/REJECTED|文件不存在|失败|error|not found|traceback/i.test(s)) return "error";
    if (/\bok\s*\(/.test(s)) return "ok";
    return "plain";
  }
  function metaLines() {
    const found = [];
    state.lines.forEach((l) => {
      if (l.t === "meta") found.push(l);
    });
    return found;
  }
  function metaState() {
    const metas = metaLines();
    if (!metas.length && !metaOf(state.current)) return null;
    const line = metas.length ? metas[metas.length - 1] : null;
    if (!line) return null;
    const scanned = metaOf(state.current);
    return {
      line,
      count: Math.max(metas.length, scanned ? scanned.count || 0 : 0),
      promptChars: String(line.text || "").length,
      promptTokenEst: estOf(line).text
    };
  }
  function callItemsOf(line, sid) {
    return (line.tool_calls || []).map((call) => {
      const fn = call.function || {};
      const name = fn.name || "(未命名工具)";
      const argsText = String(fn.arguments || "");
      const argsTokens = callTokensOf(call);
      return {
        key: call.id || "call" + line.n + "-" + (line.tool_calls || []).indexOf(call),
        call,
        id: call.id || "",
        name,
        fam: toolFamily(name),
        argsText,
        argsTokens,
        brief: toolSummary(name, fn.arguments),
        cmdLine: toolPromptLine(name, fn.arguments),
        memKey: "call." + sid + "." + call.id,
        results: [],
        attachments: [],
        outImages: [],
        previews: [],
        anchorNs: [],
        lastStatus: ""
      };
    });
  }
  function callTokensOf(call) {
    const est = call.id !== void 0 ? state.callEst[call.id] : void 0;
    return est === void 0 ? 0 : est;
  }
  function callTail(item) {
    let tail = countText(item.argsText.length, item.argsTokens);
    const last = item.results[item.results.length - 1];
    if (last) {
      const text = String(last.line.text || "");
      tail = countText(item.argsText.length, item.argsTokens) + " → " + countText(text.length, estOf(last.line).text) + (last.status === "error" ? " · error" : last.status === "ok" ? " · ok" : "");
    }
    const attachments = item.attachments.reduce((s, a) => s + (a.line.images || []).length, 0) + item.outImages.reduce((s, a) => s + (a.line.images || []).length, 0);
    if (attachments) tail += " · 附件 " + attachments + " 张（user 轮）";
    return tail;
  }
  function attachNote(a) {
    return "这一轮是 user 轮发出的（" + (a.attr && a.attr.kind === "task" ? "会话开头的原图投喂，作为任务的输入" : "工具的输入/附件") + "）· " + attributionText(a.attr);
  }
  function outImageNote(a) {
    const attr = a.attr;
    return "归属：call " + attr.callId + (attr.name ? "（" + attr.name + "，精确匹配）" : "（精确匹配）");
  }
  function imageTurnTitle(line, attr) {
    return "这一轮是 user 轮发出的（把图片投给模型），不是人打的字\n" + IMAGE_WIRE_TITLE + "\n" + attributionText(attr) + "\n" + imageTurnSummary(line);
  }
  function previewTarget(attr, callById) {
    const info = state.imgAttr;
    if (!info || !attr || !attr.callId) return null;
    const round = info.candRound ? info.candRound[attr.callId] : void 0;
    const lastId = round !== void 0 && info.roundLast ? info.roundLast[round] : "";
    return lastId && callById[lastId] || null;
  }
  function systemItem(line, sid) {
    const raw = String(line.text || "");
    return {
      type: "system",
      key: "sys" + line.n,
      line,
      raw,
      long: raw.split("\n").length > SYSTEM_PREVIEW_LINES,
      attachments: []
    };
  }
  function streamModel() {
    const attrs = imageAttributions();
    const sid = state.current ? state.current.id : "";
    const items = [];
    const callById = {};
    const taskByN = {};
    const pendingForTask = {};
    let bad = 0;
    state.lines.forEach((line) => {
      if (!line) return;
      if (line.bad) {
        bad++;
        return;
      }
      if (line.t && line.t !== "msg") return;
      if (!line.role) return;
      const isToolCall = line.role === "assistant" && line.tool_calls && line.tool_calls.length > 0;
      const isImageTurn = line.role === "user" && !!(line.images && line.images.length);
      if (state.onlyTools && line.role !== "tool" && !isToolCall && !isImageTurn) return;
      if (line.role === "user") {
        if (isImageTurn) {
          const attr = attrs[line.n] || { kind: "none", how: "", callId: "", taskLineN: 0 };
          if (attr.kind === "call" && callById[attr.callId]) {
            const host = callById[attr.callId];
            const target = previewTarget(attr, callById) || host;
            target.previews.push(line);
            if (attr.how === "call-id") {
              host.outImages.push({ line, attr });
            } else {
              host.attachments.push({ line, attr });
            }
            host.anchorNs.push(line.n);
            return;
          }
          if (attr.kind === "task" && attr.taskLineN === line.n) {
            const own2 = systemItem(line);
            own2.attachments.push({ line, attr });
            items.push(own2);
            taskByN[line.n] = own2;
            return;
          }
          if (attr.kind === "task" && attr.taskLineN) {
            const task = taskByN[attr.taskLineN];
            if (task) {
              task.attachments.push({ line, attr });
              return;
            }
            (pendingForTask[attr.taskLineN] = pendingForTask[attr.taskLineN] || []).push({ line, attr });
            return;
          }
          items.push({ type: "imageTurn", key: "img" + line.n, line, attr });
          return;
        }
        const own = systemItem(line);
        items.push(own);
        taskByN[line.n] = own;
        return;
      }
      if (line.role === "tool") {
        const node = line.tool_call_id ? callById[line.tool_call_id] : null;
        if (node) {
          const status = classifyResult(String(line.text || ""));
          node.results.push({ line, status });
          node.lastStatus = status;
          node.anchorNs.push(line.n);
          return;
        }
        items.push({ type: "result", key: "res" + line.n, line, paired: false });
        return;
      }
      if (line.role === "assistant") {
        const calls = callItemsOf(line, sid);
        calls.forEach((c) => {
          if (c.id) callById[c.id] = c;
        });
        items.push({
          type: "assistant",
          key: "asst" + line.n,
          line,
          calls,
          empty: !line.text && !line.reasoning && !(line.tool_calls || []).length
        });
        return;
      }
      items.push({ type: "other", key: "other" + line.n, line });
    });
    Object.values(callById).forEach((c) => {
      if (c.outImages.length && !c.results.length) {
        c.attachments.push(...c.outImages);
        c.outImages = [];
      }
    });
    Object.keys(pendingForTask).forEach((n) => {
      const task = taskByN[Number(n)];
      if (task) task.attachments.push(...pendingForTask[Number(n)]);
    });
    return { items, bad };
  }
  function callMsgLine() {
    const map = {};
    state.lines.forEach((line) => {
      if (!line || line.bad || line.t && line.t !== "msg") return;
      if (line.role === "assistant") {
        (line.tool_calls || []).forEach((c) => {
          if (c.id && map[c.id] === void 0) map[c.id] = line.n;
        });
      }
    });
    return map;
  }
  const _hoisted_1$d = { class: "stream-summary" };
  const _hoisted_2$c = {
    key: 0,
    class: "dot-sep"
  };
  const _sfc_main$h = /* @__PURE__ */ defineComponent({
    __name: "StreamSummary",
    setup(__props) {
      const bits = computed(() => {
        if (!state.current) return [];
        const out = [state.current.messages + " 条消息"];
        const st = aggregate(usageLines(state.lines));
        if (st) {
          out.push("输入 " + fmtTokens(st.promptTokens) + " / 输出 " + fmtTokens(st.completionTokens));
          if (st.promptTokens) out.push("缓存 " + st.cacheHitPct.toFixed(0) + "%");
          if (st.avgTtftMs) out.push("首字 " + fmtDur(st.avgTtftMs));
          const money = fmtCost(state.current.cost);
          if (money) out.push(money);
        }
        const m = metaState();
        if (m) out.push("提示词快照 " + countText(m.promptChars, m.promptTokenEst));
        return out;
      });
      const detailsBtn = computed(() => layout.cols.details > 0 ? "收起详情" : "详情 ›");
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", _hoisted_1$d, [
          (openBlock(true), createElementBlock(Fragment, null, renderList(bits.value, (b, i) => {
            return openBlock(), createElementBlock(Fragment, { key: i }, [
              i ? (openBlock(), createElementBlock("span", _hoisted_2$c)) : createCommentVNode("", true),
              createBaseVNode("span", null, toDisplayString(b), 1)
            ], 64);
          }), 128)),
          createBaseVNode("button", {
            type: "button",
            onClick: _cache[0] || (_cache[0] = //@ts-ignore
            (...args) => unref(toggleDetails) && unref(toggleDetails)(...args))
          }, toDisplayString(detailsBtn.value), 1)
        ]);
      };
    }
  });
  const StreamSummary = /* @__PURE__ */ _export_sfc(_sfc_main$h, [["__scopeId", "data-v-a0dfcd77"]]);
  const _hoisted_1$c = ["open"];
  const _hoisted_2$b = { class: "line-name" };
  const _hoisted_3$9 = ["title"];
  const _hoisted_4$6 = {
    key: 1,
    class: "line-tail"
  };
  const _sfc_main$g = /* @__PURE__ */ defineComponent({
    __name: "Disclosure",
    props: {
      cls: {},
      name: {},
      summary: {},
      tail: {},
      open: { type: Boolean }
    },
    setup(__props) {
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("details", {
          class: normalizeClass(["disclosure", __props.cls]),
          open: !!__props.open
        }, [
          createBaseVNode("summary", null, [
            _cache[1] || (_cache[1] = createBaseVNode("span", { class: "line-slot" }, [
              createBaseVNode("span", { class: "line-caret" })
            ], -1)),
            createBaseVNode("span", _hoisted_2$b, toDisplayString(__props.name), 1),
            __props.summary ? (openBlock(), createElementBlock(Fragment, { key: 0 }, [
              _cache[0] || (_cache[0] = createBaseVNode("span", { class: "line-sep" }, null, -1)),
              createBaseVNode("span", {
                class: "line-summary",
                title: __props.summary
              }, toDisplayString(__props.summary), 9, _hoisted_3$9)
            ], 64)) : createCommentVNode("", true),
            __props.tail ? (openBlock(), createElementBlock("span", _hoisted_4$6, toDisplayString(__props.tail), 1)) : createCommentVNode("", true)
          ]),
          renderSlot(_ctx.$slots, "default")
        ], 10, _hoisted_1$c);
      };
    }
  });
  const _sfc_main$f = /* @__PURE__ */ defineComponent({
    __name: "MdBody",
    props: {
      text: {}
    },
    setup(__props) {
      const props = __props;
      const host = /* @__PURE__ */ ref(null);
      function render() {
        const el2 = host.value;
        if (!el2) return;
        el2.textContent = "";
        el2.appendChild(renderMarkdown(props.text));
      }
      onMounted(render);
      watch(() => props.text, render);
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", {
          ref_key: "host",
          ref: host
        }, null, 512);
      };
    }
  });
  const _hoisted_1$b = { class: "text-wrap" };
  const _sfc_main$e = /* @__PURE__ */ defineComponent({
    __name: "FoldText",
    props: {
      text: {},
      memKey: {},
      previewLines: { default: LONG_TEXT_LINES },
      extraClass: {},
      tokens: {}
    },
    setup(__props) {
      const props = __props;
      const raw = computed(() => String(props.text ?? ""));
      const allLines = computed(() => raw.value.split("\n"));
      const long = computed(() => allLines.value.length > props.previewLines);
      const expanded = /* @__PURE__ */ ref(storeGet("text." + props.memKey) === "1");
      const label = computed(() => foldLabel(expanded.value, allLines.value.length, raw.value.length, props.tokens));
      const shown = computed(() => expanded.value || !long.value ? allLines.value.join("\n") : allLines.value.slice(0, props.previewLines).join("\n"));
      function toggle() {
        expanded.value = !expanded.value;
        storeSet("text." + props.memKey, expanded.value ? "1" : "0");
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", _hoisted_1$b, [
          unref(state).markdown ? (openBlock(), createBlock(_sfc_main$f, {
            key: 0,
            text: raw.value,
            class: normalizeClass(["md-body", __props.extraClass, { clamped: long.value && !expanded.value }]),
            style: normalizeStyle(long.value && !expanded.value ? { maxHeight: __props.previewLines * 24 + "px" } : void 0)
          }, null, 8, ["text", "class", "style"])) : (openBlock(), createElementBlock("pre", {
            key: 1,
            class: normalizeClass(["body-text", [__props.extraClass, { clamped: long.value && !expanded.value }]])
          }, toDisplayString(shown.value), 3)),
          long.value ? (openBlock(), createElementBlock("button", {
            key: 2,
            type: "button",
            class: "text-toggle",
            onClick: toggle
          }, toDisplayString(label.value), 1)) : createCommentVNode("", true)
        ]);
      };
    }
  });
  const _sfc_main$d = /* @__PURE__ */ defineComponent({
    __name: "MachineScrollBox",
    props: {
      text: {},
      memKey: {},
      tokens: {},
      error: { type: Boolean }
    },
    setup(__props) {
      const props = __props;
      const host = /* @__PURE__ */ ref(null);
      function render() {
        const el2 = host.value;
        if (!el2) return;
        el2.textContent = "";
        machineScrollInto(el2, props.text, props.memKey, void 0, props.tokens);
        if (props.error) {
          const pre = el2.querySelector(".io-text");
          if (pre) pre.setAttribute("data-error", "true");
        }
      }
      onMounted(render);
      watch(() => [props.text, props.memKey, props.tokens, props.error], render);
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", {
          ref_key: "host",
          ref: host,
          class: "text-wrap"
        }, null, 512);
      };
    }
  });
  const _hoisted_1$a = { class: "io-label" };
  const _hoisted_2$a = {
    key: 0,
    class: "io-empty"
  };
  const _hoisted_3$8 = {
    key: 2,
    class: "io-extra"
  };
  const _sfc_main$c = /* @__PURE__ */ defineComponent({
    __name: "IOSection",
    props: {
      label: {},
      text: {},
      error: { type: Boolean },
      out: { type: Boolean },
      memKey: {},
      tokens: {}
    },
    setup(__props) {
      const slots = useSlots();
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", {
          class: normalizeClass(["io-section", { "out-section": __props.out }])
        }, [
          createBaseVNode("div", _hoisted_1$a, toDisplayString(__props.label), 1),
          __props.text === void 0 || __props.text === null || __props.text === "" ? (openBlock(), createElementBlock("div", _hoisted_2$a, "（无内容）")) : (openBlock(), createBlock(_sfc_main$d, {
            key: 1,
            text: __props.text,
            "mem-key": __props.memKey || "",
            tokens: __props.tokens,
            error: __props.error
          }, null, 8, ["text", "mem-key", "tokens", "error"])),
          unref(slots).default ? (openBlock(), createElementBlock("div", _hoisted_3$8, [
            renderSlot(_ctx.$slots, "default")
          ])) : createCommentVNode("", true)
        ], 2);
      };
    }
  });
  const _hoisted_1$9 = { class: "images" };
  const _hoisted_2$9 = ["src", "alt", "title", "onClick"];
  const _sfc_main$b = /* @__PURE__ */ defineComponent({
    __name: "ImageStrip",
    props: {
      images: {}
    },
    setup(__props) {
      function show(ref2) {
        openLightbox(mediaURL(ref2), ref2);
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", _hoisted_1$9, [
          (openBlock(true), createElementBlock(Fragment, null, renderList(__props.images || [], (img) => {
            return openBlock(), createElementBlock("img", {
              key: img,
              class: "thumb",
              src: unref(mediaURL)(img),
              alt: img,
              loading: "lazy",
              title: img,
              onClick: ($event) => show(img)
            }, null, 8, _hoisted_2$9);
          }), 128))
        ]);
      };
    }
  });
  const _sfc_main$a = /* @__PURE__ */ defineComponent({
    __name: "CopyBtn",
    props: {
      text: {}
    },
    setup(__props) {
      const props = __props;
      const label = /* @__PURE__ */ ref("复制");
      let timer = 0;
      function done() {
        label.value = "已复制";
        window.clearTimeout(timer);
        timer = window.setTimeout(() => {
          label.value = "复制";
        }, 1200);
      }
      function copy() {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(props.text).then(done, () => {
            if (fallbackCopy(props.text)) done();
          });
        } else if (fallbackCopy(props.text)) {
          done();
        }
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("button", {
          type: "button",
          class: "text-toggle",
          onClick: copy
        }, toDisplayString(label.value), 1);
      };
    }
  });
  const _hoisted_1$8 = { class: "io-card" };
  const _hoisted_2$8 = {
    key: 0,
    class: "io-cmd"
  };
  const _hoisted_3$7 = { class: "cmd-line" };
  const _hoisted_4$5 = { class: "io-actions" };
  const _hoisted_5$5 = { class: "attach-note" };
  const _hoisted_6$5 = { class: "io-actions" };
  const _hoisted_7$4 = { class: "io-section attach-section" };
  const _hoisted_8$3 = { class: "attach-body" };
  const _hoisted_9$3 = { class: "attach-note" };
  const _sfc_main$9 = /* @__PURE__ */ defineComponent({
    __name: "ToolCard",
    props: {
      item: {}
    },
    setup(__props) {
      const props = __props;
      const details = /* @__PURE__ */ ref(null);
      const open = /* @__PURE__ */ ref(storeGet(props.item.memKey) === "1");
      watch(() => props.item.memKey, () => {
        open.value = storeGet(props.item.memKey) === "1";
      });
      function onToggle() {
        if (!props.item.id) return;
        const d2 = details.value && details.value.$el;
        storeSet(props.item.memKey, d2 && d2.open ? "1" : "0");
      }
      const d = computed(() => details.value ? details.value.$el : null);
      const registered = /* @__PURE__ */ new Set();
      watch(() => props.item.anchorNs.join(","), () => {
        const el2 = d.value;
        if (!el2) return;
        const want = new Set(props.item.anchorNs);
        registered.forEach((n) => {
          if (!want.has(n)) {
            unregisterAnchor(n, el2);
            registered.delete(n);
          }
        });
        props.item.anchorNs.forEach((n) => {
          if (!registered.has(n)) {
            registerAnchor(n, el2);
            registered.add(n);
          }
        });
      });
      onBeforeUnmount(() => {
        const el2 = d.value;
        if (!el2) return;
        registered.forEach((n) => unregisterAnchor(n, el2));
        registered.clear();
      });
      const tail = computed(() => callTail(props.item));
      const prettyArgs = computed(() => prettyJSON(props.item.argsText) || "(无参数)");
      return (_ctx, _cache) => {
        return openBlock(), createBlock(_sfc_main$g, {
          ref_key: "details",
          ref: details,
          cls: "disclosure-tool fam-" + __props.item.fam + (__props.item.lastStatus ? " status-" + __props.item.lastStatus : ""),
          name: __props.item.name,
          summary: __props.item.brief,
          tail: tail.value,
          open: open.value,
          "data-call-id": __props.item.id || "",
          "data-lines": __props.item.anchorNs.join(",") || void 0,
          onToggle
        }, {
          default: withCtx(() => [
            createBaseVNode("div", _hoisted_1$8, [
              __props.item.cmdLine ? (openBlock(), createElementBlock("div", _hoisted_2$8, [
                _cache[0] || (_cache[0] = createBaseVNode("span", { class: "cmd-prompt" }, "$ ", -1)),
                createBaseVNode("span", _hoisted_3$7, toDisplayString(__props.item.cmdLine), 1)
              ])) : createCommentVNode("", true),
              createVNode(_sfc_main$c, {
                label: "输入",
                text: prettyArgs.value,
                "mem-key": "in." + (__props.item.id || ""),
                tokens: __props.item.argsTokens
              }, null, 8, ["text", "mem-key", "tokens"]),
              createBaseVNode("div", _hoisted_4$5, [
                createVNode(_sfc_main$a, {
                  text: __props.item.argsText
                }, null, 8, ["text"])
              ]),
              (openBlock(true), createElementBlock(Fragment, null, renderList(__props.item.results, (r, ri) => {
                return openBlock(), createElementBlock(Fragment, {
                  key: r.line.n
                }, [
                  _cache[1] || (_cache[1] = createBaseVNode("div", { class: "io-divider" }, null, -1)),
                  createVNode(_sfc_main$c, {
                    label: "输出",
                    text: String(r.line.text || ""),
                    error: r.status === "error",
                    "mem-key": "result." + r.line.n,
                    tokens: unref(estOf)(r.line).text,
                    out: ""
                  }, {
                    default: withCtx(() => [
                      ri === 0 ? (openBlock(true), createElementBlock(Fragment, { key: 0 }, renderList(__props.item.outImages, (a) => {
                        return openBlock(), createElementBlock(Fragment, {
                          key: a.line.n
                        }, [
                          createVNode(_sfc_main$b, {
                            images: a.line.images
                          }, null, 8, ["images"]),
                          createBaseVNode("div", _hoisted_5$5, toDisplayString(unref(outImageNote)(a)), 1)
                        ], 64);
                      }), 128)) : createCommentVNode("", true)
                    ]),
                    _: 2
                  }, 1032, ["text", "error", "mem-key", "tokens"]),
                  createBaseVNode("div", _hoisted_6$5, [
                    createVNode(_sfc_main$a, {
                      text: String(r.line.text || "")
                    }, null, 8, ["text"])
                  ])
                ], 64);
              }), 128)),
              (openBlock(true), createElementBlock(Fragment, null, renderList(__props.item.attachments, (a) => {
                return openBlock(), createElementBlock(Fragment, {
                  key: a.line.n
                }, [
                  _cache[3] || (_cache[3] = createBaseVNode("div", { class: "io-divider" }, null, -1)),
                  createBaseVNode("div", _hoisted_7$4, [
                    _cache[2] || (_cache[2] = createBaseVNode("div", { class: "io-label" }, "附件（user 轮）", -1)),
                    createBaseVNode("div", _hoisted_8$3, [
                      a.line.text && a.line.n !== a.attr.taskLineN ? (openBlock(), createBlock(_sfc_main$e, {
                        key: 0,
                        text: String(a.line.text || ""),
                        "mem-key": "imgtext." + a.line.n,
                        "preview-lines": _ctx.FOLD_PREVIEW_LINES,
                        tokens: unref(estOf)(a.line).text
                      }, null, 8, ["text", "mem-key", "preview-lines", "tokens"])) : createCommentVNode("", true),
                      createVNode(_sfc_main$b, {
                        images: a.line.images
                      }, null, 8, ["images"])
                    ]),
                    createBaseVNode("div", _hoisted_9$3, toDisplayString(unref(attachNote)(a)), 1)
                  ])
                ], 64);
              }), 128))
            ])
          ]),
          _: 1
        }, 8, ["cls", "name", "summary", "tail", "open", "data-call-id", "data-lines"]);
      };
    }
  });
  const _hoisted_1$7 = { class: "thinking-body" };
  const _hoisted_2$7 = { class: "reasoning-scroll" };
  const _hoisted_3$6 = { class: "body-text reasoning-text" };
  const _hoisted_4$4 = {
    key: 0,
    class: "preview-strip"
  };
  const _hoisted_5$4 = ["src", "alt", "title", "onClick"];
  const _hoisted_6$4 = {
    key: 2,
    class: "note"
  };
  const _sfc_main$8 = /* @__PURE__ */ defineComponent({
    __name: "AssistantMsg",
    props: {
      item: {}
    },
    setup(__props) {
      function previewThumbs(call) {
        const out = [];
        call.previews.slice().sort((a, b) => a.n - b.n).forEach((l) => {
          (l.images || []).forEach((ref2) => {
            out.push({ ref: ref2, url: mediaURL(ref2) });
          });
        });
        return out;
      }
      function show(ref2, url) {
        openLightbox(url, ref2);
      }
      const props = __props;
      const line = computed(() => props.item.line);
      const thinkOpen = /* @__PURE__ */ ref(false);
      const thinkMemKey = computed(() => "thinking." + state.current.id + "." + line.value.n);
      function syncThink() {
        thinkOpen.value = storeGet(thinkMemKey.value) === "1";
      }
      syncThink();
      watch(
        [
          () => state.current && state.current.id,
          () => line.value && line.value.n,
          () => state.lines.length,
          () => state.markdown,
          () => state.onlyTools,
          () => state.unit
        ],
        syncThink
      );
      watch(() => state.forceCollapse, (on) => {
        if (on) thinkOpen.value = false;
      });
      function onThinkToggle() {
        if (state.forceCollapse) return;
        const l = line.value;
        const d = thinkRef.value && thinkRef.value.$el;
        storeSet("thinking." + state.current.id + "." + l.n, d && d.open ? "1" : "0");
      }
      const thinkRef = /* @__PURE__ */ ref(null);
      const root = /* @__PURE__ */ ref(null);
      onMounted(() => {
        if (line.value && root.value) registerAnchor(line.value.n, root.value);
      });
      onBeforeUnmount(() => {
        if (line.value && root.value) unregisterAnchor(line.value.n, root.value);
      });
      const thinkTail = computed(() => line.value && line.value.reasoning ? countText(line.value.reasoning.length, estOf(line.value).reasoning) : "");
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("section", {
          ref_key: "root",
          ref: root,
          class: "msg msg-assistant"
        }, [
          line.value.reasoning ? (openBlock(), createBlock(_sfc_main$g, {
            key: 0,
            ref_key: "thinkRef",
            ref: thinkRef,
            cls: "disclosure-thinking",
            name: "思考",
            summary: unref(firstLine)(line.value.reasoning),
            tail: thinkTail.value,
            open: thinkOpen.value,
            onToggle: onThinkToggle
          }, {
            default: withCtx(() => [
              createBaseVNode("div", _hoisted_1$7, [
                createBaseVNode("div", _hoisted_2$7, [
                  createBaseVNode("pre", _hoisted_3$6, toDisplayString(line.value.reasoning), 1)
                ])
              ])
            ]),
            _: 1
          }, 8, ["summary", "tail", "open"])) : createCommentVNode("", true),
          line.value.text ? (openBlock(), createBlock(_sfc_main$e, {
            key: 1,
            text: String(line.value.text),
            "mem-key": "asst." + line.value.n,
            "preview-lines": unref(LONG_TEXT_LINES),
            tokens: unref(estOf)(line.value).text
          }, null, 8, ["text", "mem-key", "preview-lines", "tokens"])) : createCommentVNode("", true),
          (openBlock(true), createElementBlock(Fragment, null, renderList(__props.item.calls, (c) => {
            return openBlock(), createElementBlock(Fragment, {
              key: c.key
            }, [
              createVNode(_sfc_main$9, { item: c }, null, 8, ["item"]),
              previewThumbs(c).length ? (openBlock(), createElementBlock("div", _hoisted_4$4, [
                (openBlock(true), createElementBlock(Fragment, null, renderList(previewThumbs(c), (t) => {
                  return openBlock(), createElementBlock("img", {
                    key: t.ref,
                    class: "preview-thumb",
                    src: t.url,
                    alt: t.ref,
                    loading: "lazy",
                    title: t.ref,
                    onClick: ($event) => show(t.ref, t.url)
                  }, null, 8, _hoisted_5$4);
                }), 128))
              ])) : createCommentVNode("", true)
            ], 64);
          }), 128)),
          __props.item.empty ? (openBlock(), createElementBlock("div", _hoisted_6$4, "（空消息）")) : createCommentVNode("", true)
        ], 512);
      };
    }
  });
  const AssistantMsg = /* @__PURE__ */ _export_sfc(_sfc_main$8, [["__scopeId", "data-v-0ef2183e"]]);
  const _hoisted_1$6 = { class: "sys-line" };
  const _hoisted_2$6 = { class: "line-summary" };
  const _hoisted_3$5 = {
    key: 0,
    class: "note"
  };
  const _hoisted_4$3 = {
    key: 1,
    class: "body-text sys-text"
  };
  const _hoisted_5$3 = { class: "io-section attach-section" };
  const _hoisted_6$3 = { class: "attach-body" };
  const _hoisted_7$3 = { class: "attach-note" };
  const _sfc_main$7 = /* @__PURE__ */ defineComponent({
    __name: "SystemMsg",
    props: {
      item: {}
    },
    setup(__props) {
      const props = __props;
      const line = computed(() => props.item.line);
      const raw = computed(() => String(line.value.text || ""));
      const expanded = /* @__PURE__ */ ref(storeGet("text.sys." + state.current.id + "." + line.value.n) === "1");
      const lines = computed(() => raw.value.split("\n"));
      const long = computed(() => lines.value.length > SYSTEM_PREVIEW_LINES);
      const label = computed(() => foldLabel(expanded.value, lines.value.length, raw.value.length, estOf(line.value).text));
      function toggle() {
        expanded.value = !expanded.value;
        storeSet("text.sys." + state.current.id + "." + line.value.n, expanded.value ? "1" : "0");
      }
      const root = /* @__PURE__ */ ref(null);
      onMounted(() => {
        if (line.value && root.value) registerAnchor(line.value.n, root.value);
      });
      onBeforeUnmount(() => {
        if (line.value && root.value) unregisterAnchor(line.value.n, root.value);
      });
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("section", {
          ref_key: "root",
          ref: root,
          class: "msg msg-system",
          title: "这一轮是 user 角色发出的任务提示（系统性质，不是人打的字）"
        }, [
          createBaseVNode("div", _hoisted_1$6, [
            _cache[0] || (_cache[0] = createBaseVNode("span", { class: "sys-badge" }, "系统", -1)),
            _cache[1] || (_cache[1] = createBaseVNode("span", { class: "sys-meta" }, "user 轮", -1)),
            createBaseVNode("span", _hoisted_2$6, toDisplayString(unref(firstLine)(line.value.text)), 1)
          ]),
          !raw.value ? (openBlock(), createElementBlock("div", _hoisted_3$5, "（无正文）")) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
            createBaseVNode("div", {
              class: normalizeClass(["sys-scroll", { folded: long.value && !expanded.value }]),
              style: normalizeStyle({ maxHeight: long.value && !expanded.value ? unref(SYSTEM_PREVIEW_LINES) * 24 + "px" : "var(--code-scroll-h)" })
            }, [
              unref(state).markdown ? (openBlock(), createBlock(_sfc_main$f, {
                key: 0,
                text: raw.value,
                class: "md-body sys-md"
              }, null, 8, ["text"])) : (openBlock(), createElementBlock("pre", _hoisted_4$3, toDisplayString(raw.value), 1))
            ], 6),
            long.value ? (openBlock(), createElementBlock("button", {
              key: 0,
              type: "button",
              class: "text-toggle",
              onClick: toggle
            }, toDisplayString(label.value), 1)) : createCommentVNode("", true)
          ], 64)),
          (openBlock(true), createElementBlock(Fragment, null, renderList(__props.item.attachments, (a) => {
            return openBlock(), createElementBlock(Fragment, {
              key: a.line.n
            }, [
              _cache[3] || (_cache[3] = createBaseVNode("div", { class: "io-divider" }, null, -1)),
              createBaseVNode("div", _hoisted_5$3, [
                _cache[2] || (_cache[2] = createBaseVNode("div", { class: "io-label" }, "附件（user 轮）", -1)),
                createBaseVNode("div", _hoisted_6$3, [
                  a.line.text && a.line.n !== a.attr.taskLineN ? (openBlock(), createBlock(_sfc_main$e, {
                    key: 0,
                    text: String(a.line.text || ""),
                    "mem-key": "imgtext." + a.line.n,
                    tokens: unref(estOf)(a.line).text
                  }, null, 8, ["text", "mem-key", "tokens"])) : createCommentVNode("", true),
                  createVNode(_sfc_main$b, {
                    images: a.line.images
                  }, null, 8, ["images"])
                ]),
                createBaseVNode("div", _hoisted_7$3, toDisplayString(unref(attachNote)(a)), 1)
              ])
            ], 64);
          }), 128))
        ], 512);
      };
    }
  });
  const SystemMsg = /* @__PURE__ */ _export_sfc(_sfc_main$7, [["__scopeId", "data-v-b39af669"]]);
  const _hoisted_1$5 = { class: "msg msg-image" };
  const _hoisted_2$5 = { class: "image-body" };
  const _hoisted_3$4 = { class: "attach-note" };
  const _sfc_main$6 = /* @__PURE__ */ defineComponent({
    __name: "ImageTurn",
    props: {
      item: {}
    },
    setup(__props) {
      const props = __props;
      const line = computed(() => props.item.line);
      const memKey = computed(() => "image." + state.current.id + "." + line.value.n);
      const open = /* @__PURE__ */ ref(storeGet(memKey.value) === "1");
      watch(memKey, () => {
        open.value = storeGet(memKey.value) === "1";
      });
      function onToggle() {
        const d = root.value && root.value.$el;
        storeSet(memKey.value, d && d.open ? "1" : "0");
      }
      const root = /* @__PURE__ */ ref(null);
      onMounted(() => {
        var _a;
        if (line.value && ((_a = root.value) == null ? void 0 : _a.$el)) registerAnchor(line.value.n, root.value.$el);
      });
      onBeforeUnmount(() => {
        var _a;
        if (line.value && ((_a = root.value) == null ? void 0 : _a.$el)) unregisterAnchor(line.value.n, root.value.$el);
      });
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("section", _hoisted_1$5, [
          createVNode(_sfc_main$g, {
            ref_key: "root",
            ref: root,
            cls: "disclosure-image",
            name: "图片（user 轮）",
            summary: _ctx.imageTurnSummary(line.value),
            tail: (line.value.images || []).length + " 张",
            open: open.value,
            title: unref(imageTurnTitle)(line.value, __props.item.attr),
            onToggle
          }, {
            default: withCtx(() => [
              createBaseVNode("div", _hoisted_2$5, [
                line.value.text ? (openBlock(), createBlock(_sfc_main$e, {
                  key: 0,
                  text: String(line.value.text),
                  "mem-key": "imgtext." + line.value.n,
                  tokens: unref(estOf)(line.value).text
                }, null, 8, ["text", "mem-key", "tokens"])) : createCommentVNode("", true),
                createVNode(_sfc_main$b, {
                  images: line.value.images
                }, null, 8, ["images"]),
                createBaseVNode("div", _hoisted_3$4, toDisplayString(unref(attributionText)(__props.item.attr)), 1)
              ])
            ]),
            _: 1
          }, 8, ["summary", "tail", "open", "title"])
        ]);
      };
    }
  });
  const ImageTurn = /* @__PURE__ */ _export_sfc(_sfc_main$6, [["__scopeId", "data-v-1eee90b4"]]);
  const _hoisted_1$4 = { class: "io-card" };
  const _hoisted_2$4 = { class: "io-actions" };
  const _sfc_main$5 = /* @__PURE__ */ defineComponent({
    __name: "ResultMsg",
    props: {
      item: {}
    },
    setup(__props) {
      const props = __props;
      const line = computed(() => props.item.line);
      const text = computed(() => String(line.value.text || ""));
      const status = computed(() => classifyResult(text.value));
      const root = /* @__PURE__ */ ref(null);
      onMounted(() => {
        if (line.value && root.value) registerAnchor(line.value.n, root.value);
      });
      onBeforeUnmount(() => {
        if (line.value && root.value) unregisterAnchor(line.value.n, root.value);
      });
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("section", {
          ref_key: "root",
          ref: root,
          class: "msg msg-tool"
        }, [
          createVNode(_sfc_main$g, {
            cls: "disclosure-result status-" + status.value,
            name: "(未配对的工具回执)",
            summary: unref(firstLine)(text.value),
            tail: unref(countText)(text.value.length, unref(estOf)(line.value).text) + (status.value === "error" ? " · error" : status.value === "ok" ? " · ok" : ""),
            title: !__props.item.paired ? line.value.tool_call_id ? "未找到配对的工具调用：id " + line.value.tool_call_id : "这条回执行没有 tool_call_id，无法与调用配对" : void 0
          }, {
            default: withCtx(() => [
              createBaseVNode("div", _hoisted_1$4, [
                createVNode(_sfc_main$c, {
                  label: "输出",
                  text: text.value,
                  error: status.value === "error",
                  "mem-key": "result." + line.value.n,
                  tokens: unref(estOf)(line.value).text
                }, null, 8, ["text", "error", "mem-key", "tokens"]),
                createBaseVNode("div", _hoisted_2$4, [
                  createVNode(_sfc_main$a, { text: text.value }, null, 8, ["text"])
                ])
              ])
            ]),
            _: 1
          }, 8, ["cls", "summary", "tail", "title"])
        ], 512);
      };
    }
  });
  const _hoisted_1$3 = { class: "stream" };
  const _hoisted_2$3 = {
    key: 4,
    class: "msg msg-other"
  };
  const _hoisted_3$3 = {
    key: 1,
    class: "empty"
  };
  const _sfc_main$4 = /* @__PURE__ */ defineComponent({
    __name: "Timeline",
    setup(__props) {
      const model = computed(() => streamModel());
      const emptyText = computed(() => {
        if (state.onlyTools) return "这个会话没有工具调用记录";
        if (!state.current) return "左侧选择一个会话开始浏览。";
        return "这个会话还没有可显示的消息";
      });
      onMounted(() => {
        const t = document.getElementById("timeline");
        t == null ? void 0 : t.addEventListener("scroll", () => {
          const near = t.scrollHeight - t.scrollTop - t.clientHeight < 40;
          if (!near && state.follow) state.follow = false;
        });
      });
      watch(
        [() => state.current ? state.current.id : "", () => state.lines.length],
        async (_, prev) => {
          if (prev[0] && prev[0] !== (state.current ? state.current.id : "")) {
            clearAnchors();
          }
          if (state.follow) {
            await nextTick();
            const t = document.getElementById("timeline");
            if (t) t.scrollTop = t.scrollHeight;
          }
        }
      );
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", {
          id: "timeline",
          class: normalizeClass(["timeline", { hidden: unref(state).view !== "chat" }])
        }, [
          createBaseVNode("div", _hoisted_1$3, [
            unref(state).current ? (openBlock(), createBlock(StreamSummary, { key: 0 })) : createCommentVNode("", true),
            (openBlock(true), createElementBlock(Fragment, null, renderList(model.value.items, (item) => {
              return openBlock(), createElementBlock(Fragment, {
                key: item.key
              }, [
                item.type === "assistant" ? (openBlock(), createBlock(AssistantMsg, {
                  key: 0,
                  item
                }, null, 8, ["item"])) : item.type === "system" ? (openBlock(), createBlock(SystemMsg, {
                  key: 1,
                  item
                }, null, 8, ["item"])) : item.type === "imageTurn" ? (openBlock(), createBlock(ImageTurn, {
                  key: 2,
                  item
                }, null, 8, ["item"])) : item.type === "result" ? (openBlock(), createBlock(_sfc_main$5, {
                  key: 3,
                  item
                }, null, 8, ["item"])) : (openBlock(), createElementBlock("section", _hoisted_2$3, [
                  createVNode(_sfc_main$e, {
                    text: String(item.line.text || "(无正文)"),
                    "mem-key": "other." + item.line.n,
                    "preview-lines": unref(LONG_TEXT_LINES),
                    tokens: unref(estOf)(item.line).text
                  }, null, 8, ["text", "mem-key", "preview-lines", "tokens"])
                ]))
              ], 64);
            }), 128)),
            !model.value.items.length ? (openBlock(), createElementBlock("div", _hoisted_3$3, toDisplayString(emptyText.value), 1)) : createCommentVNode("", true)
          ])
        ], 2);
      };
    }
  });
  const Timeline = /* @__PURE__ */ _export_sfc(_sfc_main$4, [["__scopeId", "data-v-7459a444"]]);
  const IMAGE_PLACEHOLDER = /\[\s*image\b|\[\s*图片|图片见|image omitted/i;
  function nextMsgLine(lines, idx) {
    for (let i = idx + 1; i < lines.length; i++) {
      if (lines[i] && !lines[i].bad && lines[i].t === "msg") return lines[i];
    }
    return null;
  }
  function callDuration(call, resultLine) {
    if (!call || !call.ts || !resultLine || !resultLine.ts) return 0;
    const t0 = Date.parse(call.ts);
    const t1 = Date.parse(resultLine.ts);
    if (isNaN(t0) || isNaN(t1) || t1 < t0) return 0;
    return t1 - t0;
  }
  function trajectoryRows() {
    const rows = [];
    const callOf = {};
    const imageAfterTool = {};
    state.lines.forEach((l) => {
      if (!l || l.bad || l.t !== "msg") return;
      if (l.role === "assistant") {
        (l.tool_calls || []).forEach((c) => {
          const fn = c.function || {};
          callOf[c.id || ""] = { name: fn.name || "?", ts: l.ts || "", n: l.n, args: String(fn.arguments || "") };
        });
      }
    });
    state.lines.forEach((line, idx) => {
      if (!line || line.bad) return;
      if (line.t === "meta") {
        rows.push({
          kind: "meta",
          tag: "元信息",
          name: line.kind || "system",
          summary: "模型 " + (line.model || "—") + " · 提示词 " + countText(String(line.text || "").length, estOf(line).text) + ((line.tools || []).length ? " · 工具 " + line.tools.length : ""),
          chars: String(line.text || "").length,
          tokens: estOf(line).text,
          status: "",
          detail: { prompt: String(line.text || "") }
        });
        return;
      }
      if (line.t === "usage") {
        const st = line.stats || {};
        rows.push({
          kind: "usage",
          tag: "用量",
          name: "请求" + (st.round ? " #" + st.round : ""),
          summary: (st.kind || "chat") + " · 输入 " + fmtTokens(st.promptTokens) + "（缓存 " + (st.cachedTokens || 0) + "）· 输出 " + fmtTokens(st.completionTokens) + (st.reasoningTokens ? "（思 " + fmtTokens(st.reasoningTokens) + "）" : ""),
          chars: "",
          tokens: 0,
          status: st.finish || "",
          time: Number(st.durationMs) || 0,
          detail: { request: JSON.stringify(st, null, 2) }
        });
        return;
      }
      if (line.t !== "msg" || !line.role) return;
      if (line.role === "user" && line.images && line.images.length) {
        const attr = imageAttributions()[line.n] || { kind: "none", how: "", callId: "", name: "", lineN: line.n, taskLineN: 0 };
        const fromTool = !!imageAfterTool[line.n];
        const callLine = attr.kind === "call" && attr.callId ? callMsgLine()[attr.callId] : void 0;
        const jumpTo = callLine !== void 0 ? callLine : attr.kind === "task" && attr.taskLineN ? attr.taskLineN : line.n;
        rows.push({
          kind: "user",
          tag: "用户",
          name: "用户",
          summary: (fromTool ? "接上一行工具回执 · " : "") + "图片 ×" + line.images.length + " · " + (firstLine(line.text) || "（无正文）") + " · " + attributionText(attr),
          title: IMAGE_WIRE_TITLE + "\n" + attributionText(attr) + (attr.how === "call-id" ? "（句柄里写了 call id，属于精确匹配）" : attr.how === "tool-name" ? "（句柄里写了工具名，按名称匹配到本轮的调用）" : attr.how === "order" ? "（旧转录没有 call id，按顺序推断；新转录会写上归属）" : attr.how === "task" ? "（这一轮带的是任务自己的图，不归任何工具调用）" : "") + (fromTool ? "\n这一轮的图片就是上一行工具回执投出来的（同一件事的两段 wire 表达，所以两行不合并）" : ""),
          chars: String(line.text || "").length,
          tokens: estOf(line).text + estOf(line).images,
          status: "",
          jump: jumpTo,
          images: line.images,
          detail: { user: String(line.text || "") || "（这一轮没有正文）" }
        });
      } else if (line.role === "user") {
        let fed = 0;
        const attrs = imageAttributions();
        Object.keys(attrs).forEach((n) => {
          const a = attrs[Number(n)];
          if (a.kind === "task" && a.taskLineN === line.n) fed++;
        });
        rows.push({
          kind: "user",
          tag: "用户",
          name: "用户",
          summary: firstLine(line.text) + (fed ? " · 附件 图片 ×" + fed : ""),
          title: fed ? "会话开头的原图投喂轮归到了这条任务（对话页里它们收在同一个块里）" : "点击跳到对话里对应的那条消息",
          chars: String(line.text || "").length,
          tokens: estOf(line).text,
          status: "",
          jump: line.n,
          detail: { user: String(line.text || "") }
        });
      } else if (line.role === "assistant") {
        if (line.reasoning) {
          rows.push({
            kind: "think",
            tag: "思考",
            name: "reasoning",
            summary: firstLine(line.reasoning),
            chars: line.reasoning.length,
            tokens: estOf(line).reasoning,
            status: "",
            jump: line.n,
            detail: { thinking: line.reasoning }
          });
        }
        if (line.text) {
          rows.push({
            kind: "msg",
            tag: "助手",
            name: "AI",
            summary: firstLine(line.text),
            chars: line.text.length,
            tokens: estOf(line).text,
            status: "",
            jump: line.n,
            detail: { message: String(line.text) }
          });
        }
        (line.tool_calls || []).forEach((c, i) => {
          const fn = c.function || {};
          const name = fn.name || "(未命名工具)";
          const args = String(fn.arguments || "");
          rows.push({
            kind: "tool",
            tag: "工具",
            name,
            summary: toolSummary(name, fn.arguments),
            chars: args.length,
            tokens: estOf(line).calls[i] || 0,
            status: "",
            jump: line.n,
            detail: { input: prettyJSON(args) || args }
          });
        });
      } else if (line.role === "tool") {
        const info = line.tool_call_id ? callOf[line.tool_call_id] || null : null;
        const text = String(line.text || "");
        const after = nextMsgLine(state.lines, idx);
        const imageNext = !!(after && after.role === "user" && after.images && after.images.length);
        if (imageNext) imageAfterTool[after.n] = true;
        const hint = imageNext ? IMAGE_PLACEHOLDER.test(text) ? " · 图片见下一行用户轮" : " · 图片在下一行用户轮里" : "";
        rows.push({
          kind: "result",
          tag: "结果",
          name: info ? info.name : "(未配对的工具回执)",
          summary: firstLine(text) + hint,
          chars: text.length,
          tokens: estOf(line).text,
          title: imageNext ? "这一行是工具回执：tool 消息的 content 只能是文本，随行的图片被回灌在紧随其后的 user 轮里（两行是同一件事，保持两行不合并）" : "点击跳到对话里对应的那条消息",
          status: classifyResult(text),
          jump: line.n,
          time: callDuration(info, line),
          detail: { output: text }
        });
      }
    });
    rows.forEach((r, i) => {
      r.rid = r.kind + "@" + (r.jump ?? "x") + "#" + i;
    });
    return rows;
  }
  const TRAJ_KINDS = [
    { id: "user", label: "用户" },
    { id: "msg", label: "助手" },
    { id: "think", label: "思考" },
    { id: "tool", label: "工具" },
    { id: "result", label: "结果" },
    { id: "meta", label: "元信息" },
    { id: "usage", label: "用量" }
  ];
  function jumpToLine(n) {
    const node = anchorOf(n);
    switchView("chat");
    if (!node) return;
    node.scrollIntoView({ block: "center" });
    node.classList.remove("flash");
    void node.offsetWidth;
    node.classList.add("flash");
  }
  const _sfc_main$3 = /* @__PURE__ */ defineComponent({
    __name: "MachineText",
    props: {
      text: {},
      cls: {},
      tag: { default: "div" }
    },
    setup(__props) {
      const props = __props;
      const host = /* @__PURE__ */ ref(null);
      function render() {
        const el2 = host.value;
        if (!el2) return;
        el2.textContent = "";
        el2.appendChild(machineBlock(props.text, props.cls));
      }
      onMounted(render);
      watch(() => [props.text, props.cls], render);
      return (_ctx, _cache) => {
        return openBlock(), createBlock(resolveDynamicComponent(__props.tag), {
          ref_key: "host",
          ref: host
        }, null, 512);
      };
    }
  });
  const _hoisted_1$2 = { class: "traj-toolbar" };
  const _hoisted_2$2 = { class: "traj-toolbar-inner" };
  const _hoisted_3$2 = { class: "traj-filters" };
  const _hoisted_4$2 = ["aria-pressed"];
  const _hoisted_5$2 = ["title", "aria-pressed", "onClick"];
  const _hoisted_6$2 = { class: "traj-count" };
  const _hoisted_7$2 = { class: "traj-scroll" };
  const _hoisted_8$2 = {
    key: 0,
    class: "traj-empty"
  };
  const _hoisted_9$2 = {
    key: 1,
    class: "traj-table"
  };
  const _hoisted_10$2 = { class: "num-head" };
  const _hoisted_11$2 = ["data-kind", "data-error", "title", "onClick"];
  const _hoisted_12$2 = { class: "traj-num" };
  const _hoisted_13$2 = ["onClick"];
  const _hoisted_14$2 = ["onClick"];
  const _hoisted_15$2 = { class: "traj-name" };
  const _hoisted_16$2 = ["title"];
  const _hoisted_17$2 = { class: "traj-num-cell" };
  const _hoisted_18$2 = { class: "traj-num-cell" };
  const _hoisted_19$2 = {
    key: 0,
    class: "traj-detail"
  };
  const _hoisted_20$2 = { colspan: 7 };
  const _hoisted_21$2 = { class: "traj-detail-inner" };
  const _hoisted_22$2 = { class: "traj-detail-title" };
  const _hoisted_23$2 = {
    key: 1,
    class: "code"
  };
  const _hoisted_24$2 = { class: "row-actions" };
  const _sfc_main$2 = /* @__PURE__ */ defineComponent({
    __name: "Trajectory",
    setup(__props) {
      const rows = computed(() => state.current ? trajectoryRows() : []);
      const visible = computed(() => rows.value.filter(trajVisible));
      const kinds = computed(
        () => TRAJ_KINDS.map((k) => ({ ...k, count: rows.value.filter((r) => r.kind === k.id).length })).filter((k) => k.count > 0)
      );
      const allPressed = computed(() => Object.keys(state.trajKinds).length === 0);
      function trajVisible(row) {
        const picked = Object.keys(state.trajKinds).filter((k) => state.trajKinds[k]);
        if (!picked.length) return true;
        return picked.indexOf(row.kind) >= 0;
      }
      function pick(k) {
        if (state.trajKinds[k]) delete state.trajKinds[k];
        else state.trajKinds[k] = true;
      }
      function sizeCell(row) {
        return state.unit === "char" ? row.chars ? String(row.chars) : "—" : row.tokens ? countValue(row.chars, row.tokens) : "—";
      }
      function statusCell(row) {
        if (row.status === "error") return { cls: "error", text: "✗ error" };
        if (row.status === "ok") return { cls: "ok", text: "✓ ok" };
        return { cls: "plain", text: row.status === "plain" || !row.status ? "—" : row.status };
      }
      const DETAIL_LABELS = {
        prompt: "系统提示词",
        thinking: "思考",
        user: "用户消息",
        message: "助手消息",
        input: "输入",
        output: "输出",
        request: "用量行"
      };
      function detailKind(key) {
        return key === "input" || key === "request" || key === "output" ? "code" : "plain";
      }
      function rowKey(row, _idx) {
        return row.rid;
      }
      function selectRow(row) {
        if (!row.jump) return;
        state.trajSelected = state.trajSelected === row.rid ? null : row.rid;
      }
      function goChat(row) {
        if (!row.jump) return;
        state.trajSelected = row.rid;
        jumpToLine(row.jump);
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", {
          id: "trajectory",
          class: normalizeClass(["trajectory", { hidden: unref(state).view === "chat" }])
        }, [
          createBaseVNode("div", _hoisted_1$2, [
            createBaseVNode("div", _hoisted_2$2, [
              createBaseVNode("div", _hoisted_3$2, [
                createBaseVNode("button", {
                  type: "button",
                  class: "traj-chip",
                  "aria-pressed": allPressed.value ? "true" : "false",
                  onClick: _cache[0] || (_cache[0] = ($event) => unref(state).trajKinds = {})
                }, "全部", 8, _hoisted_4$2),
                (openBlock(true), createElementBlock(Fragment, null, renderList(kinds.value, (k) => {
                  return openBlock(), createElementBlock("button", {
                    key: k.id,
                    type: "button",
                    class: "traj-chip",
                    title: "只看 / 不看「" + k.label + "」",
                    "aria-pressed": unref(state).trajKinds[k.id] ? "true" : "false",
                    onClick: ($event) => pick(k.id)
                  }, toDisplayString(k.label) + " " + toDisplayString(k.count), 9, _hoisted_5$2);
                }), 128))
              ]),
              createBaseVNode("span", _hoisted_6$2, toDisplayString(visible.value.length) + " / " + toDisplayString(rows.value.length) + " 步", 1)
            ])
          ]),
          createBaseVNode("div", _hoisted_7$2, [
            !visible.value.length ? (openBlock(), createElementBlock("div", _hoisted_8$2, toDisplayString(rows.value.length ? "当前筛选没有匹配的步骤" : unref(state).current ? "这个会话还没有步骤" : "左侧选择一个会话后，这里列出它的全部步骤。"), 1)) : (openBlock(), createElementBlock("table", _hoisted_9$2, [
              _cache[7] || (_cache[7] = createBaseVNode("colgroup", null, [
                createBaseVNode("col", { class: "col-n" }),
                createBaseVNode("col", { class: "col-kind" }),
                createBaseVNode("col", { class: "col-name" }),
                createBaseVNode("col"),
                createBaseVNode("col", { class: "col-status" }),
                createBaseVNode("col", { class: "col-size" }),
                createBaseVNode("col", { class: "col-time" })
              ], -1)),
              createBaseVNode("thead", null, [
                createBaseVNode("tr", null, [
                  _cache[1] || (_cache[1] = createBaseVNode("th", { class: "num-head" }, "#", -1)),
                  _cache[2] || (_cache[2] = createBaseVNode("th", null, "类型", -1)),
                  _cache[3] || (_cache[3] = createBaseVNode("th", null, "名称", -1)),
                  _cache[4] || (_cache[4] = createBaseVNode("th", null, "摘要", -1)),
                  _cache[5] || (_cache[5] = createBaseVNode("th", null, "状态", -1)),
                  createBaseVNode("th", _hoisted_10$2, toDisplayString(unref(unitLabel)()), 1),
                  _cache[6] || (_cache[6] = createBaseVNode("th", { class: "num-head" }, "耗时", -1))
                ])
              ]),
              createBaseVNode("tbody", null, [
                (openBlock(true), createElementBlock(Fragment, null, renderList(visible.value, (row, idx) => {
                  return openBlock(), createElementBlock(Fragment, {
                    key: rowKey(row)
                  }, [
                    createBaseVNode("tr", {
                      class: normalizeClass(["traj-row", { selected: row.jump && unref(state).trajSelected === row.rid }]),
                      "data-kind": row.kind,
                      "data-error": row.status === "error" ? "true" : void 0,
                      title: row.title || "点击在右侧详情栏看这一步",
                      onClick: ($event) => row.jump ? selectRow(row) : unref(state).trajOpen[row.rid] = !unref(state).trajOpen[row.rid]
                    }, [
                      createBaseVNode("td", _hoisted_12$2, [
                        createBaseVNode("button", {
                          type: "button",
                          class: "traj-disclose",
                          title: "展开完整输入输出",
                          onClick: withModifiers(($event) => unref(state).trajOpen[row.rid] = !unref(state).trajOpen[row.rid], ["stop"])
                        }, toDisplayString(unref(state).trajOpen[row.rid] ? "▾" : "▸"), 9, _hoisted_13$2),
                        createTextVNode(toDisplayString(row.jump ? String(row.jump) : "—") + " ", 1),
                        row.jump ? (openBlock(), createElementBlock("button", {
                          key: 0,
                          type: "button",
                          class: "traj-gochat",
                          title: "跳到对话里对应的那条消息",
                          onClick: withModifiers(($event) => goChat(row), ["stop"])
                        }, "↳", 8, _hoisted_14$2)) : createCommentVNode("", true)
                      ]),
                      createBaseVNode("td", null, [
                        createBaseVNode("span", {
                          class: normalizeClass(["kind-tag", row.status === "error" ? "kind-error" : "kind-" + row.kind])
                        }, toDisplayString(row.tag), 3)
                      ]),
                      createBaseVNode("td", _hoisted_15$2, toDisplayString(row.name), 1),
                      createBaseVNode("td", {
                        class: "traj-summary",
                        title: row.summary || ""
                      }, toDisplayString(row.summary || "—"), 9, _hoisted_16$2),
                      createBaseVNode("td", {
                        class: normalizeClass("traj-status " + statusCell(row).cls)
                      }, toDisplayString(statusCell(row).text), 3),
                      createBaseVNode("td", _hoisted_17$2, toDisplayString(sizeCell(row)), 1),
                      createBaseVNode("td", _hoisted_18$2, toDisplayString(row.time ? unref(fmtDur)(row.time) : "—"), 1)
                    ], 10, _hoisted_11$2),
                    unref(state).trajOpen[row.rid] ? (openBlock(), createElementBlock("tr", _hoisted_19$2, [
                      createBaseVNode("td", _hoisted_20$2, [
                        createBaseVNode("div", _hoisted_21$2, [
                          (openBlock(true), createElementBlock(Fragment, null, renderList(row.detail || {}, (text, key) => {
                            return openBlock(), createElementBlock("div", { key }, [
                              createBaseVNode("div", _hoisted_22$2, toDisplayString(DETAIL_LABELS[key] || key), 1),
                              detailKind(key) === "code" ? (openBlock(), createBlock(_sfc_main$3, {
                                key: 0,
                                text: String(text || ""),
                                cls: "code"
                              }, null, 8, ["text"])) : (openBlock(), createElementBlock("pre", _hoisted_23$2, toDisplayString(String(text || "")), 1)),
                              createBaseVNode("div", _hoisted_24$2, [
                                createVNode(_sfc_main$a, {
                                  text: String(text || "")
                                }, null, 8, ["text"])
                              ])
                            ]);
                          }), 128)),
                          row.images && row.images.length ? (openBlock(), createBlock(_sfc_main$b, {
                            key: 0,
                            images: row.images
                          }, null, 8, ["images"])) : createCommentVNode("", true)
                        ])
                      ])
                    ])) : createCommentVNode("", true)
                  ], 64);
                }), 128))
              ])
            ]))
          ])
        ], 2);
      };
    }
  });
  const Trajectory = /* @__PURE__ */ _export_sfc(_sfc_main$2, [["__scopeId", "data-v-0d6b1c96"]]);
  function currentEstimate() {
    const id = state.current ? state.current.id : "";
    const list = state.sessions || [];
    for (let i = 0; i < list.length; i++) {
      if (list[i].id === id && list[i].estimate) return list[i].estimate;
    }
    return state.current && state.current.estimate || null;
  }
  function imageEstimate(lines) {
    const sum = { count: 0, tokens: 0 };
    (lines || []).forEach((line) => {
      const est = estOf(line);
      sum.count += est.imageCount || 0;
      sum.tokens += est.images || 0;
    });
    return sum;
  }
  function sessionKVRows() {
    const cur = state.current;
    if (!cur) return [];
    const rows = [
      { k: "项目", v: cur.project || projectOf(cur.id), title: cur.id },
      { k: "阶段", v: cur.stageTitle || cur.stage || "—" },
      { k: "会话", md: sessionTitleOf(cur), title: cur.id, mono: true },
      { k: "文件", v: cur.name || "—", title: cur.path, mono: true },
      { k: "消息", v: cur.messages + " 条 · " + state.lines.length + " 行" },
      { k: "大小", v: fmtSize(cur.size) },
      { k: "最后写入", v: fmtClock(cur.mtime) }
    ];
    if (cur.imageName) rows.push({ k: "图片", v: imageDisplayName(cur), title: imageTipText(cur), mono: true });
    if (cur.imageFile) rows.push({ k: "图片文件", v: cur.imageFile, title: cur.imageName, mono: true });
    if (cur.imagePath) rows.push({ k: "图片路径", v: cur.imagePath, mono: true });
    if (cur.imageName && cur.page) rows.push({ k: "页码", v: "第 " + cur.page + " 页" });
    if (cur.imageName && cur.imageOrder) rows.push({ k: "顺序", v: "书内第 " + cur.imageOrder + " 张" });
    if (cur.imageName && cur.imageType) rows.push({ k: "类型", v: cur.imageType });
    if (cur.imageName && cur.imageCaption) rows.push({ k: "图注", v: cur.imageCaption });
    return rows;
  }
  function sessionSubPath() {
    const cur = state.current;
    return cur ? subPathOf(cur.id) : "";
  }
  function statsModel() {
    const st = aggregate(usageLines(state.lines));
    if (!st) return null;
    const sub = st.requests + " 次请求 · " + (st.streamed ? "流式" : "非流式") + (st.spanMs ? " · 会话跨度 " + fmtDur(st.spanMs) : "");
    const tiles = [];
    tiles.push({
      label: "输入 tokens",
      value: fmtTokens(st.promptTokens),
      title: st.promptTokens + " prompt tokens（含缓存命中 " + st.cachedTokens + "）\n厂商实测值：随请求发出的图片 token 已经包含在里面，不单列。"
    });
    const imgs = imageEstimate(state.lines);
    if (imgs.count) {
      const est = currentEstimate();
      const per = Math.round(imgs.tokens / imgs.count);
      const rule = est ? est.rule : "本地估算";
      const lines = [imgs.count + " 张图片的本地估算合计 ≈ " + fmtTokens(imgs.tokens) + "（每张 ≈ " + fmtTokens(per) + "）", "估算口径：" + rule];
      let value = "≈ " + fmtTokens(per) + "/张";
      if (est && est.measuredPerImage) {
        value = "实测 " + fmtTokens(est.measuredPerImage) + "/张";
        lines.push("实测 " + fmtTokens(est.measuredPerImage) + "/张：厂商 prompt_tokens 的相邻差值推出的每张均值" + (est.measuredSamples ? "（" + est.measuredSamples + " 步 / " + est.measuredImages + " 张）" : ""));
        lines.push("本地估算每张 ≈ " + fmtTokens(per) + "（口径：" + rule + "，本会话合计 ≈ " + fmtTokens(imgs.tokens) + "）");
      } else {
        lines.push("没有可用的实测样本：本会话的用量行还不足以推出每张实测值（无用量行、或没有一次请求新增图片）");
      }
      lines.push("对照：上面的「输入 tokens」是厂商实测的 prompt_tokens，其中已经包含图片 token。");
      tiles.push({ label: "图片 " + imgs.count + " 张", value, title: lines.join("\n") });
    }
    tiles.push({
      label: "缓存命中",
      value: st.promptTokens ? st.cacheHitPct.toFixed(0) + "%" : "—",
      title: "前缀缓存命中率 = Σcached_tokens / Σprompt_tokens（供应商未上报时为 —）"
    });
    tiles.push({
      label: "输出 tokens",
      value: fmtTokens(st.completionTokens),
      title: st.completionTokens + " completion tokens" + (st.reasoningTokens ? "，其中思考 " + st.reasoningTokens : "")
    });
    tiles.push({ label: "平均首字", value: st.avgTtftMs ? fmtDur(st.avgTtftMs) : "—", title: '每请求"发出→第一个流式增量"的平均耗时' });
    tiles.push({
      label: "输出速度",
      value: (st.outputTps || 0).toFixed(1) + " tok/s",
      title: "生成速度 = Σ输出 tokens / Σ(请求耗时 − 首字延迟)，不含排队与思考等待"
    });
    tiles.push({ label: "平均耗时", value: fmtDur(st.avgDurationMs), title: "每请求平均墙钟耗时（含思考与工具执行前后的等待）" });
    if (st.reasoningTokens) tiles.push({ label: "思考 tokens", value: fmtTokens(st.reasoningTokens), title: "reasoning_tokens（思考链）" });
    const sessionCost = state.current && state.current.cost;
    if (sessionCost) {
      tiles.push({
        label: "费用",
        value: fmtCost(sessionCost),
        title: '按配置里的 models.*.price 计算：未命中缓存的输入 × input + 命中缓存的输入 × cached + 输出 × output。没配价格的模型不显示金额（¥0 会被读成"没花钱"）。'
      });
    }
    const rows = st.perRequest.map((l) => {
      const one = l.stats;
      const kindLabel = one.kind === "compact" ? "上下文压缩摘要请求" : one.kind === "nudge" ? "空回复后的强制文本请求" : "普通对话回合";
      return {
        cells: [
          one.round ? "#" + one.round : "—",
          one.ttftMs ? fmtDur(one.ttftMs) : "—",
          one.durationMs ? fmtDur(one.durationMs) : "—",
          fmtTokens(one.promptTokens) + (one.promptTokens ? " · " + (one.cachedTokens * 100 / one.promptTokens).toFixed(0) + "%" : ""),
          fmtTokens(one.completionTokens)
        ],
        title: (l.ts ? fmtClock(l.ts) + "\n" : "") + kindLabel + (one.kind ? "（kind=" + one.kind + "）" : "") + (one.model ? "\n模型 " + one.model : "") + "\n输入 " + one.promptTokens + " tokens（缓存命中 " + one.cachedTokens + "）\n输出 " + one.completionTokens + " tokens" + (one.reasoningTokens ? "（其中思考 " + one.reasoningTokens + "）" : "") + "\n输出速度 " + (one.outputTps || 0).toFixed(1) + " tok/s\n结束原因 " + (one.finish || "—")
      };
    });
    return { sub, tiles, cols: ["回合", "首字", "耗时", "输入·缓存", "输出"], rows, rowCount: st.requests };
  }
  function toolSchemas(line) {
    return (line.tools || []).map((t) => ({
      name: t.name || "(未命名工具)",
      desc: t.description ? String(t.description) : "",
      params: t.parameters ? String(t.parameters) : "",
      descCount: t.description ? "描述 " + countText(String(t.description).length, t.descTokens) : "",
      paramCount: t.parameters ? "schema " + countText(String(t.parameters).length, t.paramTokens) : ""
    }));
  }
  function metaCardKey(line) {
    return "meta." + (state.current ? state.current.id : "") + "." + (line.system_sha || line.n);
  }
  function metaModel() {
    const m = metaState();
    if (!m) return null;
    const line = m.line;
    const bits = ["模型 " + (line.model || "—")];
    if (line.session_label) bits.push("会话 " + line.session_label);
    bits.push("sha " + shortSHA(line.system_sha));
    bits.push(countText(m.promptChars, m.promptTokenEst));
    const promptText = String(line.text || "（这条 meta 行没有正文）");
    let mode = "plain";
    if (jsonPretty(promptText) !== null) mode = "json";
    else if (state.markdown) mode = "md";
    const tools = toolSchemas(line);
    return {
      key: metaCardKey(line),
      open: storeGet(metaCardKey(line)) === "1",
      bits: bits.join(" · "),
      promptText,
      copyText: String(line.text || ""),
      mode,
      tools,
      toolsHead: tools.length ? "工具定义 " + tools.length + " 个（parameters 的 JSON 默认收起）" : "工具定义 0 个",
      countNote: m.count > 1 ? "共 " + m.count + " 条，显示最新" : ""
    };
  }
  const META_COUNT_TIP = "同一个转录里有 {n} 条 meta 行（多次运行 / 提示词变化各一条），这里显示最后一条。";
  const _hoisted_1$1 = {
    id: "details-body",
    class: "details-body"
  };
  const _hoisted_2$1 = {
    key: 0,
    class: "note"
  };
  const _hoisted_3$1 = {
    key: 0,
    class: "detail-block traj-step"
  };
  const _hoisted_4$1 = { class: "detail-block-title" };
  const _hoisted_5$1 = { class: "traj-step-head" };
  const _hoisted_6$1 = { class: "kind-tag" };
  const _hoisted_7$1 = { class: "traj-step-name" };
  const _hoisted_8$1 = { class: "traj-detail-title" };
  const _hoisted_9$1 = {
    key: 1,
    class: "code"
  };
  const _hoisted_10$1 = { class: "row-actions" };
  const _hoisted_11$1 = { class: "detail-block" };
  const _hoisted_12$1 = { class: "detail-kv" };
  const _hoisted_13$1 = ["title"];
  const _hoisted_14$1 = {
    key: 0,
    class: "stats-sub"
  };
  const _hoisted_15$1 = {
    key: 1,
    class: "detail-block"
  };
  const _hoisted_16$1 = { class: "stats-sub" };
  const _hoisted_17$1 = { class: "tiles" };
  const _hoisted_18$1 = ["title"];
  const _hoisted_19$1 = { class: "tile-value" };
  const _hoisted_20$1 = { class: "tile-label" };
  const _hoisted_21$1 = {
    class: "stats-details",
    open: ""
  };
  const _hoisted_22$1 = { class: "schema-head" };
  const _hoisted_23$1 = { class: "schema-meta" };
  const _hoisted_24$1 = { class: "stats-scroll" };
  const _hoisted_25$1 = { class: "stats-table" };
  const _hoisted_26$1 = ["title"];
  const _hoisted_27$1 = {
    key: 2,
    class: "detail-block"
  };
  const _hoisted_28$1 = ["open"];
  const _hoisted_29 = { class: "line-summary" };
  const _hoisted_30 = { class: "schema-body" };
  const _hoisted_31 = { class: "prompt-scroll" };
  const _hoisted_32 = { class: "row-actions" };
  const _hoisted_33 = { class: "meta-tools" };
  const _hoisted_34 = { class: "meta-tools-head" };
  const _hoisted_35 = {
    key: 0,
    class: "note"
  };
  const _hoisted_36 = { class: "schema-head" };
  const _hoisted_37 = { class: "schema-index" };
  const _hoisted_38 = { class: "schema-name" };
  const _hoisted_39 = {
    key: 0,
    class: "schema-meta"
  };
  const _hoisted_40 = {
    key: 1,
    class: "schema-meta"
  };
  const _hoisted_41 = { class: "schema-body" };
  const _hoisted_42 = {
    key: 0,
    class: "body-text schema-desc"
  };
  const _hoisted_43 = {
    key: 1,
    class: "schema-params"
  };
  const _hoisted_44 = { class: "schema-body" };
  const _hoisted_45 = { class: "row-actions" };
  const _hoisted_46 = {
    key: 2,
    class: "note"
  };
  const _hoisted_47 = ["title"];
  const _hoisted_48 = {
    key: 3,
    class: "note"
  };
  const _sfc_main$1 = /* @__PURE__ */ defineComponent({
    __name: "DetailsPanel",
    setup(__props) {
      const kvRows = computed(() => sessionKVRows());
      const subPath = computed(() => sessionSubPath());
      const stats = computed(() => statsModel());
      const meta = computed(() => metaModel());
      const metaOpen = /* @__PURE__ */ ref(false);
      watch(
        () => {
          var _a;
          return (_a = meta.value) == null ? void 0 : _a.key;
        },
        () => {
          var _a;
          metaOpen.value = !!((_a = meta.value) == null ? void 0 : _a.open);
        },
        { immediate: true }
      );
      function onMetaToggle(ev) {
        const m = meta.value;
        if (!m) return;
        const open = ev.target.open;
        metaOpen.value = open;
        storeSet(m.key, open ? "1" : "0");
      }
      const countTip = computed(
        () => {
          var _a;
          return ((_a = meta.value) == null ? void 0 : _a.countNote) ? META_COUNT_TIP.replace("{n}", meta.value.countNote.replace(/^共 (\d+) 条.*$/, "$1")) : "";
        }
      );
      const TRAJ_STEP_LABELS = {
        prompt: "系统提示词",
        thinking: "思考",
        user: "用户消息",
        message: "助手消息",
        input: "输入",
        output: "输出",
        request: "用量行"
      };
      const trajStep = computed(() => {
        if (state.view !== "trajectory" || state.trajSelected == null || !state.current) return null;
        const row = trajectoryRows().find((r) => r.rid === state.trajSelected);
        return row ? { n: row.jump, tag: row.tag, name: row.name, detail: row.detail || {}, images: row.images || [] } : null;
      });
      function trajDetailKind(key) {
        return key === "input" || key === "request" || key === "output" ? "code" : "plain";
      }
      function closeTrajStep() {
        state.trajSelected = null;
      }
      return (_ctx, _cache) => {
        return openBlock(), createElementBlock("div", _hoisted_1$1, [
          !unref(state).current ? (openBlock(), createElementBlock("div", _hoisted_2$1, "左侧选择一个会话后，这里显示它的指标与元信息。")) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
            trajStep.value ? (openBlock(), createElementBlock("section", _hoisted_3$1, [
              createBaseVNode("h3", _hoisted_4$1, [
                createTextVNode(" 步骤 #" + toDisplayString(trajStep.value.n) + " ", 1),
                createBaseVNode("button", {
                  type: "button",
                  class: "icon-btn traj-step-close",
                  title: "关闭这一步的详情",
                  onClick: closeTrajStep
                }, "×")
              ]),
              createBaseVNode("div", _hoisted_5$1, [
                createBaseVNode("span", _hoisted_6$1, toDisplayString(trajStep.value.tag), 1),
                createBaseVNode("span", _hoisted_7$1, toDisplayString(trajStep.value.name), 1)
              ]),
              (openBlock(true), createElementBlock(Fragment, null, renderList(trajStep.value.detail, (text, key) => {
                return openBlock(), createElementBlock("div", {
                  key,
                  class: "traj-step-section"
                }, [
                  createBaseVNode("div", _hoisted_8$1, toDisplayString(TRAJ_STEP_LABELS[key] || key), 1),
                  trajDetailKind(key) === "code" ? (openBlock(), createBlock(_sfc_main$3, {
                    key: 0,
                    text: String(text || ""),
                    cls: "code"
                  }, null, 8, ["text"])) : (openBlock(), createElementBlock("pre", _hoisted_9$1, toDisplayString(String(text || "")), 1)),
                  createBaseVNode("div", _hoisted_10$1, [
                    createVNode(_sfc_main$a, {
                      text: String(text || "")
                    }, null, 8, ["text"])
                  ])
                ]);
              }), 128)),
              trajStep.value.images.length ? (openBlock(), createBlock(_sfc_main$b, {
                key: 0,
                images: trajStep.value.images
              }, null, 8, ["images"])) : createCommentVNode("", true)
            ])) : createCommentVNode("", true),
            createBaseVNode("section", _hoisted_11$1, [
              _cache[0] || (_cache[0] = createBaseVNode("h3", { class: "detail-block-title" }, "会话", -1)),
              createBaseVNode("dl", _hoisted_12$1, [
                (openBlock(true), createElementBlock(Fragment, null, renderList(kvRows.value, (row) => {
                  return openBlock(), createElementBlock(Fragment, {
                    key: row.k
                  }, [
                    createBaseVNode("dt", null, toDisplayString(row.k), 1),
                    createBaseVNode("dd", {
                      class: normalizeClass(row.mono ? "mono" : void 0),
                      title: row.title || void 0
                    }, [
                      row.md ? (openBlock(), createBlock(_sfc_main$j, {
                        key: 0,
                        text: row.md,
                        tag: "span",
                        class: "detail-name"
                      }, null, 8, ["text"])) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
                        createTextVNode(toDisplayString(row.v), 1)
                      ], 64))
                    ], 10, _hoisted_13$1)
                  ], 64);
                }), 128))
              ]),
              subPath.value ? (openBlock(), createElementBlock("div", _hoisted_14$1, "目录：" + toDisplayString(subPath.value), 1)) : createCommentVNode("", true)
            ]),
            stats.value ? (openBlock(), createElementBlock("section", _hoisted_15$1, [
              _cache[2] || (_cache[2] = createBaseVNode("h3", { class: "detail-block-title" }, "指标", -1)),
              createBaseVNode("div", _hoisted_16$1, toDisplayString(stats.value.sub), 1),
              createBaseVNode("div", _hoisted_17$1, [
                (openBlock(true), createElementBlock(Fragment, null, renderList(stats.value.tiles, (t) => {
                  return openBlock(), createElementBlock("div", {
                    key: t.label,
                    class: "tile",
                    title: t.title || void 0
                  }, [
                    createBaseVNode("div", _hoisted_19$1, toDisplayString(t.value), 1),
                    createBaseVNode("div", _hoisted_20$1, toDisplayString(t.label), 1)
                  ], 8, _hoisted_18$1);
                }), 128))
              ]),
              createBaseVNode("details", _hoisted_21$1, [
                createBaseVNode("summary", _hoisted_22$1, [
                  _cache[1] || (_cache[1] = createBaseVNode("span", { class: "schema-name" }, "每次请求明细", -1)),
                  createBaseVNode("span", _hoisted_23$1, toDisplayString(stats.value.rowCount) + " 行", 1)
                ]),
                createBaseVNode("div", _hoisted_24$1, [
                  createBaseVNode("table", _hoisted_25$1, [
                    createBaseVNode("tr", null, [
                      (openBlock(true), createElementBlock(Fragment, null, renderList(stats.value.cols, (h) => {
                        return openBlock(), createElementBlock("th", { key: h }, toDisplayString(h), 1);
                      }), 128))
                    ]),
                    (openBlock(true), createElementBlock(Fragment, null, renderList(stats.value.rows, (r, i) => {
                      return openBlock(), createElementBlock("tr", {
                        key: i,
                        class: "req-row",
                        title: r.title
                      }, [
                        (openBlock(true), createElementBlock(Fragment, null, renderList(r.cells, (c, j) => {
                          return openBlock(), createElementBlock("td", { key: j }, toDisplayString(c), 1);
                        }), 128))
                      ], 8, _hoisted_26$1);
                    }), 128))
                  ])
                ])
              ])
            ])) : createCommentVNode("", true),
            meta.value ? (openBlock(), createElementBlock("section", _hoisted_27$1, [
              _cache[7] || (_cache[7] = createBaseVNode("h3", { class: "detail-block-title" }, "元信息", -1)),
              createBaseVNode("details", {
                class: "disclosure meta-card",
                open: metaOpen.value,
                onToggle: onMetaToggle
              }, [
                createBaseVNode("summary", null, [
                  _cache[3] || (_cache[3] = createBaseVNode("span", { class: "line-slot" }, [
                    createBaseVNode("span", { class: "line-caret" })
                  ], -1)),
                  _cache[4] || (_cache[4] = createBaseVNode("span", { class: "line-name" }, "系统提示词（本次运行快照，不参与回放）", -1)),
                  _cache[5] || (_cache[5] = createBaseVNode("span", { class: "line-sep" }, null, -1)),
                  createBaseVNode("span", _hoisted_29, toDisplayString(meta.value.bits), 1)
                ]),
                createBaseVNode("div", _hoisted_30, [
                  createBaseVNode("div", _hoisted_31, [
                    meta.value.mode === "json" ? (openBlock(), createBlock(_sfc_main$3, {
                      key: 0,
                      text: meta.value.promptText,
                      cls: "body-text prompt-text"
                    }, null, 8, ["text"])) : meta.value.mode === "md" ? (openBlock(), createBlock(_sfc_main$f, {
                      key: 1,
                      text: meta.value.promptText,
                      class: "md-body prompt-md"
                    }, null, 8, ["text"])) : (openBlock(), createBlock(_sfc_main$3, {
                      key: 2,
                      text: meta.value.promptText,
                      cls: "body-text prompt-text"
                    }, null, 8, ["text"]))
                  ]),
                  createBaseVNode("div", _hoisted_32, [
                    createVNode(_sfc_main$a, {
                      text: meta.value.copyText
                    }, null, 8, ["text"])
                  ]),
                  createBaseVNode("div", _hoisted_33, [
                    createBaseVNode("div", _hoisted_34, toDisplayString(meta.value.toolsHead), 1),
                    !meta.value.tools.length ? (openBlock(), createElementBlock("div", _hoisted_35, "这条 meta 行没有记录工具定义。")) : createCommentVNode("", true),
                    (openBlock(true), createElementBlock(Fragment, null, renderList(meta.value.tools, (t, i) => {
                      return openBlock(), createElementBlock("details", {
                        key: i,
                        class: "tool-schema"
                      }, [
                        createBaseVNode("summary", _hoisted_36, [
                          createBaseVNode("span", _hoisted_37, "#" + toDisplayString(i + 1), 1),
                          createBaseVNode("span", _hoisted_38, toDisplayString(t.name), 1),
                          t.descCount ? (openBlock(), createElementBlock("span", _hoisted_39, toDisplayString(t.descCount), 1)) : createCommentVNode("", true),
                          t.paramCount ? (openBlock(), createElementBlock("span", _hoisted_40, toDisplayString(t.paramCount), 1)) : createCommentVNode("", true)
                        ]),
                        createBaseVNode("div", _hoisted_41, [
                          t.desc ? (openBlock(), createElementBlock("pre", _hoisted_42, toDisplayString(t.desc), 1)) : createCommentVNode("", true),
                          t.params ? (openBlock(), createElementBlock("details", _hoisted_43, [
                            _cache[6] || (_cache[6] = createBaseVNode("summary", { class: "schema-head" }, [
                              createBaseVNode("span", { class: "schema-name" }, "parameters"),
                              createBaseVNode("span", { class: "schema-meta" }, "JSON · 默认收起")
                            ], -1)),
                            createBaseVNode("div", _hoisted_44, [
                              createVNode(_sfc_main$3, {
                                text: t.params,
                                cls: "code"
                              }, null, 8, ["text"]),
                              createBaseVNode("div", _hoisted_45, [
                                createVNode(_sfc_main$a, {
                                  text: t.params
                                }, null, 8, ["text"])
                              ])
                            ])
                          ])) : (openBlock(), createElementBlock("div", _hoisted_46, "（这条工具定义没有记录 parameters）"))
                        ])
                      ]);
                    }), 128))
                  ]),
                  meta.value.countNote ? (openBlock(), createElementBlock("div", {
                    key: 0,
                    class: "note",
                    title: countTip.value
                  }, toDisplayString(meta.value.countNote), 9, _hoisted_47)) : createCommentVNode("", true)
                ])
              ], 40, _hoisted_28$1)
            ])) : createCommentVNode("", true),
            !kvRows.value.length && !stats.value && !meta.value ? (openBlock(), createElementBlock("div", _hoisted_48, "这个会话没有可显示的详情。")) : createCommentVNode("", true)
          ], 64))
        ]);
      };
    }
  });
  const DetailsPanel = /* @__PURE__ */ _export_sfc(_sfc_main$1, [["__scopeId", "data-v-fbc1d95d"]]);
  const _hoisted_1 = ["data-sidebar-collapsed", "data-details-collapsed", "data-dragging"];
  const _hoisted_2 = ["data-dragging"];
  const _hoisted_3 = {
    id: "center-col",
    class: "center-col"
  };
  const _hoisted_4 = {
    id: "center-header",
    class: "center-header"
  };
  const _hoisted_5 = { class: "title-row" };
  const _hoisted_6 = ["aria-pressed"];
  const _hoisted_7 = {
    id: "crumbs",
    class: "crumbs",
    "aria-label": "面包屑"
  };
  const _hoisted_8 = {
    key: 0,
    class: "crumb crumb-current"
  };
  const _hoisted_9 = {
    id: "header-actions",
    class: "header-actions"
  };
  const _hoisted_10 = {
    key: 0,
    id: "mode-badge",
    class: "badge badge-live"
  };
  const _hoisted_11 = {
    key: 1,
    class: "badge"
  };
  const _hoisted_12 = ["title", "aria-pressed"];
  const _hoisted_13 = { class: "tabs-row" };
  const _hoisted_14 = {
    class: "tabs",
    role: "tablist",
    "aria-label": "视图"
  };
  const _hoisted_15 = ["aria-selected"];
  const _hoisted_16 = ["aria-selected"];
  const _hoisted_17 = {
    class: "tab-tools",
    role: "group",
    "aria-label": "显示选项"
  };
  const _hoisted_18 = ["aria-pressed"];
  const _hoisted_19 = ["aria-pressed"];
  const _hoisted_20 = ["aria-pressed"];
  const _hoisted_21 = ["aria-pressed", "title"];
  const _hoisted_22 = ["aria-pressed", "title"];
  const _hoisted_23 = ["aria-pressed", "title"];
  const _hoisted_24 = { class: "view-area" };
  const _hoisted_25 = ["data-dragging", "data-hidden"];
  const _hoisted_26 = {
    id: "details-col",
    class: "details-col",
    "aria-label": "详情"
  };
  const _hoisted_27 = { class: "details-head" };
  const _hoisted_28 = ["src", "alt"];
  const _sfc_main = /* @__PURE__ */ defineComponent({
    __name: "App",
    setup(__props) {
      const frame = /* @__PURE__ */ ref(null);
      const drag = /* @__PURE__ */ ref(null);
      let dragOrigin = 0;
      let dragBase = 0;
      function onDragStart(ev, side) {
        var _a, _b;
        ev.preventDefault();
        dragOrigin = ev.clientX;
        dragBase = side === "sidebar" ? layout.cols.sidebar : layout.cols.details;
        (_b = (_a = ev.currentTarget).setPointerCapture) == null ? void 0 : _b.call(_a, ev.pointerId);
        drag.value = { side };
      }
      function onDragMove(ev) {
        if (!drag.value) return;
        const dx = ev.clientX - dragOrigin;
        if (drag.value.side === "sidebar") {
          state.sidebar = clampWidth(dragBase + dx, SIDEBAR_MIN, SIDEBAR_MAX);
          if (state.narrow) state.narrowExpanded = true;
        } else {
          state.details = clampWidth(dragBase - dx, DETAILS_MIN, DETAILS_MAX);
        }
        applyLayout();
      }
      function onDragEnd() {
        if (!drag.value) return;
        drag.value = null;
        persistLayout();
      }
      function onDragDblClick(side) {
        if (side === "sidebar") state.sidebar = SIDEBAR_DEFAULT;
        else state.details = DETAILS_DEFAULT;
        persistLayout();
        applyLayout();
      }
      function onClickFollow() {
        state.follow = !state.follow;
        if (state.follow) scrollToBottom();
      }
      function onClickCollapseThinking() {
        state.forceCollapse = !state.forceCollapse;
      }
      function onClickOnlyTools() {
        state.onlyTools = !state.onlyTools;
      }
      function onClickMarkdown() {
        state.markdown = !state.markdown;
        storeSet("markdown", state.markdown ? "1" : "0");
      }
      function onClickUnit() {
        state.unit = state.unit === "char" ? "token" : "char";
        storeSet("unit", state.unit);
      }
      function scrollToBottom() {
        window.setTimeout(() => {
          const el2 = document.getElementById("timeline");
          if (el2) el2.scrollTop = el2.scrollHeight;
        }, 0);
      }
      const badgeText = computed(() => state.polling ? "实时" : "实时（已断开）");
      const badCount = computed(() => state.lines.reduce((n, l) => n + (l && l.bad ? 1 : 0), 0));
      const headerSummary = computed(() => {
        const cur = state.current;
        if (!cur) return "";
        const parts = [];
        parts.push(cur.messages + " 条消息");
        if (state.lines.length) parts.push(state.lines.length + " 行");
        parts.push(fmtSize(cur.size));
        if (badCount.value) parts.push("坏行 " + badCount.value);
        return parts.join(" · ");
      });
      const curProject = computed(() => state.current ? state.current.project || projectOf(state.current.id) : "");
      const sessionTitle = computed(() => state.current ? sessionTitleOf(state.current) : "");
      const bannerText = computed(() => {
        if (badCount.value > 0) {
          return "已跳过 " + badCount.value + " 行坏数据（无法解析为 JSON，可能是一次写入中途读到的不完整行）";
        }
        return state.pullError;
      });
      function onKeydown(ev) {
        var _a, _b;
        const tag = (_a = ev.target) == null ? void 0 : _a.tagName;
        const typing = tag === "INPUT" || tag === "TEXTAREA" || ((_b = ev.target) == null ? void 0 : _b.isContentEditable);
        if (ev.key === "Escape") closeLightbox();
        if (typing) return;
        if (ev.key === "[") toggleSidebar();
        if (ev.key === "]") toggleDetails();
      }
      let lbDrag = null;
      function onLbWheel(ev) {
        ev.preventDefault();
        const target = ev.currentTarget.querySelector("#lightbox-img");
        if (!target) return;
        zoomLightbox(ev.deltaY > 0 ? 0.9 : 1 / 0.9, ev.clientX, ev.clientY, target.getBoundingClientRect());
      }
      function onLbPointerdown(ev) {
        if (ev.button !== 0) return;
        lbDrag = { x: ev.clientX, y: ev.clientY, tx: lightbox.tx, ty: lightbox.ty, moved: false };
        ev.currentTarget.setPointerCapture(ev.pointerId);
      }
      function onLbPointermove(ev) {
        if (!lbDrag) return;
        const dx = ev.clientX - lbDrag.x;
        const dy = ev.clientY - lbDrag.y;
        if (!lbDrag.moved && Math.hypot(dx, dy) > 3) lbDrag.moved = true;
        if (lbDrag.moved) {
          lightbox.tx = lbDrag.tx + dx;
          lightbox.ty = lbDrag.ty + dy;
        }
      }
      function onLbPointerup() {
        lbDrag = null;
      }
      function onLbClick() {
        if (!lbDrag) closeLightbox();
      }
      function onLbDblclick(ev) {
        ev.preventDefault();
        resetLightbox();
      }
      let ro = null;
      let pollTimer = 0;
      onMounted(() => {
        loadState();
        applyTheme(storedTheme());
        const el2 = frame.value;
        if (el2) {
          layout.viewport = el2.clientWidth || window.innerWidth;
          if (window.ResizeObserver) {
            ro = new ResizeObserver(() => {
              layout.viewport = el2.clientWidth || window.innerWidth;
            });
            ro.observe(el2);
          } else {
            window.addEventListener("resize", onResize);
          }
        }
        document.addEventListener("keydown", onKeydown);
        bootData();
        pollTimer = window.setInterval(() => {
          if (!document.hidden) void refreshIndex();
        }, 2e3);
      });
      function onResize() {
        const el2 = frame.value;
        if (el2) layout.viewport = el2.clientWidth || window.innerWidth;
      }
      watchEffect(() => {
        void layout.viewport;
        void state.sidebar;
        void state.details;
        void state.narrowExpanded;
        applyLayout();
      });
      onBeforeUnmount(() => {
        ro == null ? void 0 : ro.disconnect();
        if (pollTimer) window.clearInterval(pollTimer);
        document.removeEventListener("keydown", onKeydown);
      });
      return (_ctx, _cache) => {
        var _a, _b;
        return openBlock(), createElementBlock(Fragment, null, [
          createBaseVNode("div", {
            id: "frame",
            ref_key: "frame",
            ref: frame,
            class: "frame",
            style: normalizeStyle({ gridTemplateColumns: `${unref(layout).cols.sidebar}px minmax(0, 1fr) ${unref(layout).cols.details}px` }),
            "data-sidebar-collapsed": unref(layout).sidebarCollapsed ? "" : void 0,
            "data-details-collapsed": unref(layout).detailsCollapsed ? "" : void 0,
            "data-dragging": drag.value ? "" : void 0
          }, [
            createVNode(Sidebar),
            createBaseVNode("div", {
              id: "handle-sidebar",
              class: "handle",
              "data-side": "sidebar",
              role: "separator",
              "aria-orientation": "vertical",
              "aria-label": "调整侧栏宽度",
              style: normalizeStyle({ left: `${unref(layout).cols.sidebar}px` }),
              "data-dragging": ((_a = drag.value) == null ? void 0 : _a.side) === "sidebar" ? "true" : void 0,
              onPointerdown: _cache[0] || (_cache[0] = ($event) => onDragStart($event, "sidebar")),
              onPointermove: onDragMove,
              onPointerup: onDragEnd,
              onPointercancel: onDragEnd,
              onDblclick: _cache[1] || (_cache[1] = ($event) => onDragDblClick("sidebar"))
            }, null, 44, _hoisted_2),
            createBaseVNode("main", _hoisted_3, [
              createBaseVNode("header", _hoisted_4, [
                createBaseVNode("div", _hoisted_5, [
                  createBaseVNode("button", {
                    id: "side-toggle",
                    class: "icon-btn",
                    type: "button",
                    title: "折叠 / 展开侧栏",
                    "aria-label": "折叠或展开侧栏",
                    "aria-pressed": unref(layout).sidebarCollapsed ? "false" : "true",
                    onClick: _cache[2] || (_cache[2] = //@ts-ignore
                    (...args) => unref(toggleSidebar) && unref(toggleSidebar)(...args))
                  }, "▤", 8, _hoisted_6),
                  createBaseVNode("nav", _hoisted_7, [
                    !unref(state).current ? (openBlock(), createElementBlock("span", _hoisted_8, "未选择会话")) : (openBlock(), createElementBlock(Fragment, { key: 1 }, [
                      createBaseVNode("button", {
                        class: "crumb is-link",
                        type: "button",
                        title: "在侧栏里定位到这个项目",
                        onClick: _cache[3] || (_cache[3] = ($event) => unref(revealProject)(curProject.value))
                      }, toDisplayString(curProject.value), 1),
                      _cache[11] || (_cache[11] = createBaseVNode("span", { class: "crumb-sep" }, "›", -1)),
                      createVNode(_sfc_main$j, {
                        text: sessionTitle.value,
                        tag: "span",
                        class: "crumb crumb-current",
                        title: unref(state).current.id
                      }, null, 8, ["text", "title"])
                    ], 64))
                  ]),
                  createBaseVNode("div", _hoisted_9, [
                    !unref(state).current ? (openBlock(), createElementBlock("span", _hoisted_10, toDisplayString(badgeText.value), 1)) : (openBlock(), createElementBlock("span", _hoisted_11, toDisplayString(headerSummary.value), 1))
                  ]),
                  createBaseVNode("button", {
                    id: "details-toggle",
                    class: "icon-btn",
                    type: "button",
                    title: unref(layout).detailsCollapsed ? "详情面板（元信息 / 指标）" : "关闭详情面板",
                    "aria-label": "展开或收起详情面板",
                    "aria-pressed": unref(layout).detailsCollapsed ? "false" : "true",
                    onClick: _cache[4] || (_cache[4] = //@ts-ignore
                    (...args) => unref(toggleDetails) && unref(toggleDetails)(...args))
                  }, "ⓘ", 8, _hoisted_12)
                ]),
                createBaseVNode("div", _hoisted_13, [
                  createBaseVNode("div", _hoisted_14, [
                    createBaseVNode("button", {
                      id: "tab-chat",
                      class: normalizeClass(["tab", { "tab-active": unref(state).view === "chat" }]),
                      type: "button",
                      role: "tab",
                      "aria-selected": unref(state).view === "chat" ? "true" : "false",
                      "data-view": "chat",
                      onClick: _cache[5] || (_cache[5] = ($event) => unref(switchView)("chat"))
                    }, "对话", 10, _hoisted_15),
                    createBaseVNode("button", {
                      id: "tab-traj",
                      class: normalizeClass(["tab", { "tab-active": unref(state).view === "trajectory" }]),
                      type: "button",
                      role: "tab",
                      "aria-selected": unref(state).view === "trajectory" ? "true" : "false",
                      "data-view": "trajectory",
                      onClick: _cache[6] || (_cache[6] = ($event) => unref(switchView)("trajectory"))
                    }, "轨迹", 10, _hoisted_16)
                  ]),
                  createBaseVNode("div", _hoisted_17, [
                    createBaseVNode("button", {
                      id: "follow",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).follow ? "true" : "false",
                      title: "新消息到达时自动滚动到底部",
                      onClick: onClickFollow
                    }, "自动跟随", 8, _hoisted_18),
                    createBaseVNode("button", {
                      id: "collapse-thinking",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).forceCollapse ? "true" : "false",
                      title: "把所有消息的思考过程折叠起来",
                      onClick: onClickCollapseThinking
                    }, "折叠全部思考", 8, _hoisted_19),
                    createBaseVNode("button", {
                      id: "only-tools",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).onlyTools ? "true" : "false",
                      title: "只显示工具调用与工具结果",
                      onClick: onClickOnlyTools
                    }, "仅看工具调用", 8, _hoisted_20),
                    createBaseVNode("button", {
                      id: "md-toggle",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).markdown ? "true" : "false",
                      title: unref(state).markdown ? "消息正文按 Markdown 渲染（标题 / 列表 / 代码块 / 表格），点击回到纯文本" : "消息正文按纯文本显示（pre-wrap），点击改用 Markdown 渲染",
                      onClick: onClickMarkdown
                    }, "Markdown", 8, _hoisted_21),
                    createBaseVNode("button", {
                      id: "unit-toggle",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).unit === "char" ? "false" : "true",
                      title: unref(state).unit === "char" ? "计数按字符数显示（精确值），点击改为 token" : "计数按 token 显示（本地估算，带 ≈），点击改为字符",
                      onClick: onClickUnit
                    }, toDisplayString(unref(state).unit === "char" ? "字符" : "token"), 9, _hoisted_22),
                    createBaseVNode("button", {
                      id: "theme-toggle",
                      class: "tab-toggle",
                      type: "button",
                      "aria-pressed": unref(state).theme === "dark" ? "true" : "false",
                      title: unref(state).theme === "dark" ? "切换为白天模式（浅色，默认）" : "切换为夜间模式（深色）",
                      onClick: _cache[7] || (_cache[7] = //@ts-ignore
                      (...args) => unref(toggleTheme) && unref(toggleTheme)(...args))
                    }, toDisplayString(unref(state).theme === "dark" ? "☀️ 浅色" : "🌙 深色"), 9, _hoisted_23)
                  ])
                ])
              ]),
              createBaseVNode("div", {
                id: "banner",
                class: normalizeClass(["banner", { hidden: !bannerText.value }])
              }, toDisplayString(bannerText.value), 3),
              createBaseVNode("div", _hoisted_24, [
                createVNode(Timeline),
                createVNode(Trajectory)
              ])
            ]),
            createBaseVNode("div", {
              id: "handle-details",
              class: "handle",
              "data-side": "details",
              role: "separator",
              "aria-orientation": "vertical",
              "aria-label": "调整详情栏宽度",
              style: normalizeStyle({ left: `${Math.max(0, unref(layout).viewport - unref(layout).cols.details)}px` }),
              "data-dragging": ((_b = drag.value) == null ? void 0 : _b.side) === "details" ? "true" : void 0,
              "data-hidden": unref(layout).detailsCollapsed ? "true" : void 0,
              onPointerdown: _cache[8] || (_cache[8] = ($event) => onDragStart($event, "details")),
              onPointermove: onDragMove,
              onPointerup: onDragEnd,
              onPointercancel: onDragEnd,
              onDblclick: _cache[9] || (_cache[9] = ($event) => onDragDblClick("details"))
            }, null, 44, _hoisted_25),
            createBaseVNode("aside", _hoisted_26, [
              createBaseVNode("div", _hoisted_27, [
                _cache[12] || (_cache[12] = createBaseVNode("span", { class: "details-title" }, "详情", -1)),
                createBaseVNode("button", {
                  id: "details-close",
                  class: "icon-btn",
                  type: "button",
                  title: "关闭详情面板",
                  "aria-label": "关闭详情面板",
                  onClick: _cache[10] || (_cache[10] = ($event) => unref(state).details > 0 && unref(toggleDetails)())
                }, "✕")
              ]),
              createVNode(DetailsPanel)
            ])
          ], 12, _hoisted_1),
          createBaseVNode("div", {
            id: "lightbox",
            class: normalizeClass(["lightbox", { hidden: !unref(lightbox).open }]),
            onClick: onLbClick,
            onWheel: onLbWheel,
            onPointerdown: onLbPointerdown,
            onPointermove: onLbPointermove,
            onPointerup: onLbPointerup,
            onPointercancel: onLbPointerup,
            onDblclick: onLbDblclick
          }, [
            createBaseVNode("img", {
              id: "lightbox-img",
              src: unref(lightbox).open ? unref(lightbox).url : void 0,
              alt: unref(lightbox).ref,
              style: normalizeStyle({ transform: "translate(" + unref(lightbox).tx + "px," + unref(lightbox).ty + "px) scale(" + unref(lightbox).scale + ")" })
            }, null, 12, _hoisted_28),
            _cache[13] || (_cache[13] = createBaseVNode("div", { class: "lightbox-hint" }, "滚轮缩放 · 拖动平移 · 双击复位 · 点击空白或 Esc 关闭", -1))
          ], 34)
        ], 64);
      };
    }
  });
  const App = /* @__PURE__ */ _export_sfc(_sfc_main, [["__scopeId", "data-v-92016ca1"]]);
  createApp(App).mount("#app");
})();
