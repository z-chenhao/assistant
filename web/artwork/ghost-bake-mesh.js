/* One continuous textured mesh; torso vertices remain fixed. Private demo rig. */
window.createStewardGhostMesh = async () => {
  const canvas = document.createElement('canvas');
  canvas.className = 'ghost-mesh';
  canvas.setAttribute('aria-hidden', 'true');
  const gl = canvas.getContext('webgl', { alpha: true, antialias: true, premultipliedAlpha: false, preserveDrawingBuffer: true });
  if (!gl) throw new Error('WebGL unavailable');
  let contextLost = false;
  canvas.addEventListener('webglcontextlost', event => {
    event.preventDefault();
    contextLost = true;
  });

  const vertexSource = `
    attribute vec2 uv;
    uniform float angle;
    varying vec2 sampleUV;
    void main() {
      float boundary = .70 + (uv.y - .38) * (5.0 / 18.0);
      float arm = smoothstep(boundary, boundary + .065, uv.x);
      arm *= 1.0 - smoothstep(.46, .57, uv.y);
      vec2 pivot = vec2(.75, .54);
      vec2 offset = uv - pivot;
      float s = sin(angle), c = cos(angle);
      vec2 rotated = pivot + vec2(c * offset.x - s * offset.y, s * offset.x + c * offset.y);
      vec2 position = mix(uv, rotated, arm);
      gl_Position = vec4(position.x * 2.0 - 1.0, 1.0 - position.y * 2.0, 0.0, 1.0);
      sampleUV = uv;
    }
  `;
  const fragmentSource = `
    precision mediump float;
    uniform sampler2D charcoal;
    uniform sampler2D ivory;
    uniform float lightBody;
    uniform float eyeOpen;
    uniform float feather;
    varying vec2 sampleUV;
    float eye(vec2 p, vec2 center) {
      vec2 q = p - center;
      float c = .9396926, s = .3420201;
      q = vec2(c*q.x-s*q.y, s*q.x+c*q.y);
      q.y /= max(.08, eyeOpen);
      float distance = length(vec2(q.x, max(abs(q.y)-31.0, 0.0)))-29.0;
      return 1.0-smoothstep(-feather, feather, distance);
    }
    vec2 smile(float t) {
      float u=1.0-t;
      return u*u*u*vec2(681,521)+3.0*u*u*t*vec2(695,544)+3.0*u*t*t*vec2(723,539)+t*t*t*vec2(730,512);
    }
    float mouth(vec2 p) {
      float distance=1000.0;
      vec2 a=smile(0.0);
      for(int i=1;i<=12;i++) {
        vec2 b=smile(float(i)/12.0), v=b-a;
        float t=clamp(dot(p-a,v)/dot(v,v),0.0,1.0);
        distance=min(distance,length(p-a-t*v));
        a=b;
      }
      return 1.0-smoothstep(7.0-feather,7.0+feather,distance);
    }
    void main() {
      vec4 darkPixel = texture2D(charcoal, sampleUV);
      vec4 lightPixel = texture2D(ivory, sampleUV);
      lightPixel.a *= darkPixel.a;
      vec4 body=mix(darkPixel,lightPixel,lightBody);
      vec2 p=sampleUV*1254.0;
      float face=max(eye(p,vec2(593,443)),eye(p,vec2(734,410)));
      if(p.x>660.0 && p.x<750.0 && p.y>490.0 && p.y<565.0) face=max(face,mouth(p));
      vec3 ink=mix(vec3(.965,.961,.945),vec3(.137),lightBody);
      body.rgb=mix(body.rgb,ink,face);
      vec2 ground=(sampleUV-vec2(.495,.915))/vec2(.215,.015);
      float shadow=0.0;
      vec3 shadowColor=mix(vec3(.13),vec3(.02),lightBody);
      float alpha=body.a+shadow*(1.0-body.a);
      vec3 color=(body.rgb*body.a+shadowColor*shadow*(1.0-body.a))/max(alpha,.001);
      gl_FragColor=vec4(color,alpha);
    }
  `;
  function shader(type, source) {
    const item = gl.createShader(type);
    gl.shaderSource(item, source);
    gl.compileShader(item);
    if (!gl.getShaderParameter(item, gl.COMPILE_STATUS)) throw new Error('Avatar shader failed');
    return item;
  }
  const program = gl.createProgram();
  gl.attachShader(program, shader(gl.VERTEX_SHADER, vertexSource));
  gl.attachShader(program, shader(gl.FRAGMENT_SHADER, fragmentSource));
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) throw new Error('Avatar mesh failed');
  gl.useProgram(program);

  const segments = 96;
  const vertices = [];
  const indices = [];
  for (let y = 0; y <= segments; y++) {
    for (let x = 0; x <= segments; x++) vertices.push(x / segments, y / segments);
  }
  for (let y = 0; y < segments; y++) {
    for (let x = 0; x < segments; x++) {
      const a = y * (segments + 1) + x;
      indices.push(a, a + 1, a + segments + 1, a + 1, a + segments + 2, a + segments + 1);
    }
  }
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(vertices), gl.STATIC_DRAW);
  const location = gl.getAttribLocation(program, 'uv');
  gl.enableVertexAttribArray(location);
  gl.vertexAttribPointer(location, 2, gl.FLOAT, false, 0, 0);
  gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, new Uint16Array(indices), gl.STATIC_DRAW);

  async function texture(path, unit, name) {
    const image = new Image();
    image.src = path;
    await image.decode();
    // Runtime prefiltering for a power-of-two GPU texture; source PNG is unchanged.
    const surface = document.createElement('canvas');
    surface.width = surface.height = 1024;
    const painter = surface.getContext('2d');
    painter.drawImage(image, 0, 0, 1024, 1024);
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, gl.createTexture());
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, surface);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
    gl.generateMipmap(gl.TEXTURE_2D);
    gl.uniform1i(gl.getUniformLocation(program, name), unit);
  }
  await texture('ghost-charcoal.png', 0, 'charcoal');
  await texture('ghost-ivory.png', 1, 'ivory');
  const angleUniform = gl.getUniformLocation(program, 'angle');
  const styleUniform = gl.getUniformLocation(program, 'lightBody');
  const eyeUniform = gl.getUniformLocation(program, 'eyeOpen');
  const featherUniform = gl.getUniformLocation(program, 'feather');
  return {
    canvas,
    draw(width, angle, isIvory, eyeOpen = 1) {
      if (contextLost || gl.isContextLost()) return false;
      const pixels = Math.max(1, Math.min(960, Math.round(width * devicePixelRatio)));
      if (canvas.width !== pixels || canvas.height !== pixels) canvas.width = canvas.height = pixels;
      gl.viewport(0, 0, pixels, pixels);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.uniform1f(angleUniform, angle);
      gl.uniform1f(styleUniform, isIvory ? 1 : 0);
      gl.uniform1f(eyeUniform, eyeOpen);
      gl.uniform1f(featherUniform, 1254 / pixels);
      gl.drawElements(gl.TRIANGLES, indices.length, gl.UNSIGNED_SHORT, 0);
      return true;
    }
  };
};
