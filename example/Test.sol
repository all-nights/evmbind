// SPDX-License-Identifier: MIT

pragma solidity ^0.8.9;

library Test {
		function foo() public pure returns (uint) {
				return 42;
		}

		function mod(uint a, uint b) public pure returns (uint) {
				return a % b;
		}

		function customAddress() public view returns (address) {
			return address(bytes20(keccak256(abi.encodePacked(block.timestamp))));
		}
}
