import React from "react";

export const Logo = ({
  className = "w-6 h-6",
  ...props
}: React.SVGProps<SVGSVGElement>) => {
  const gradientPrefix = React.useId().replace(/:/g, "");
  const ribbonTop = `${gradientPrefix}-ribbon-top`;
  const ribbonBottom = `${gradientPrefix}-ribbon-bottom`;
  const ribbonLeft = `${gradientPrefix}-ribbon-left`;

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 192 192"
      fill="none"
      className={className}
      {...props}
    >
      <defs>
        <linearGradient
          id={ribbonTop}
          x1="83"
          y1="29"
          x2="164"
          y2="132"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#60E5E4" />
          <stop offset="0.48" stopColor="#54A8F4" />
          <stop offset="1" stopColor="#D77CEB" />
        </linearGradient>
        <linearGradient
          id={ribbonBottom}
          x1="158"
          y1="128"
          x2="37"
          y2="146"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#A875EE" />
          <stop offset="0.52" stopColor="#817CF0" />
          <stop offset="1" stopColor="#27CBE5" />
        </linearGradient>
        <linearGradient
          id={ribbonLeft}
          x1="39"
          y1="141"
          x2="91"
          y2="25"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#20C9DE" />
          <stop offset="0.5" stopColor="#68E2E4" />
          <stop offset="1" stopColor="#4AA7F3" />
        </linearGradient>
      </defs>
      <g>
        <path
          d="M58 112C37 79 48 35 79 18C99 7 123 15 134 34C148 59 139 91 129 112Q117 118 105 99C113 80 121 57 111 41C104 30 91 28 81 35C60 49 60 77 81 99Q68 118 58 112Z"
          fill={`url(#${ribbonTop})`}
          opacity="0.8"
        />
        <path
          d="M58 112C37 79 48 35 79 18C99 7 123 15 134 34C148 59 139 91 129 112Q117 118 105 99C113 80 121 57 111 41C104 30 91 28 81 35C60 49 60 77 81 99Q68 118 58 112Z"
          fill={`url(#${ribbonBottom})`}
          opacity="0.78"
          transform="rotate(120 96 96)"
        />
        <path
          d="M58 112C37 79 48 35 79 18C99 7 123 15 134 34C148 59 139 91 129 112Q117 118 105 99C113 80 121 57 111 41C104 30 91 28 81 35C60 49 60 77 81 99Q68 118 58 112Z"
          fill={`url(#${ribbonLeft})`}
          opacity="0.78"
          transform="rotate(240 96 96)"
        />
      </g>
    </svg>
  );
};

export const BubblesLoading = ({
  className = "",
  ...props
}: React.SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 32 24"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <circle cx="0" cy="12" r="0" transform="translate(8 0)" fill="currentColor">
      <animate
        attributeName="r"
        begin="0"
        calcMode="spline"
        dur="1.2s"
        keySplines="0.2 0.2 0.4 0.8;0.2 0.6 0.4 0.8;0.2 0.6 0.4 0.8"
        keyTimes="0;0.2;0.7;1"
        repeatCount="indefinite"
        values="0; 4; 0; 0"
      />
    </circle>
    <circle
      cx="0"
      cy="12"
      r="0"
      transform="translate(16 0)"
      fill="currentColor"
    >
      <animate
        attributeName="r"
        begin="0.3"
        calcMode="spline"
        dur="1.2s"
        keySplines="0.2 0.2 0.4 0.8;0.2 0.6 0.4 0.8;0.2 0.6 0.4 0.8"
        keyTimes="0;0.2;0.7;1"
        repeatCount="indefinite"
        values="0; 4; 0; 0"
      />
    </circle>
    <circle
      cx="0"
      cy="12"
      r="0"
      transform="translate(24 0)"
      fill="currentColor"
    >
      <animate
        attributeName="r"
        begin="0.6"
        calcMode="spline"
        dur="1.2s"
        keySplines="0.2 0.2 0.4 0.8;0.2 0.6 0.4 0.8;0.2 0.6 0.4 0.8"
        keyTimes="0;0.2;0.7;1"
        repeatCount="indefinite"
        values="0; 4; 0; 0"
      />
    </circle>
  </svg>
);
