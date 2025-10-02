// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/**
 * @title VulnerableApp
 * @dev Example vulnerable application that trusts txGasPayer() for access control
 * This demonstrates the real-world impact of the fee delegation context abuse vulnerability
 */

interface IExtensionV2 {
    function txGasPayer() external view returns (address);
}

contract VulnerableApp {
    // VeChain Extension contract address (builtin)
    IExtensionV2 constant EXTENSION = IExtensionV2(0x0000000000000000000000457874656E73696F6E);
    
    // Trusted sponsor who should have access to premium services
    address public trustedSponsor;
    
    // Premium service token balance
    mapping(address => uint256) public premiumBalances;
    
    // Events
    event PremiumAccess(address indexed user, address indexed gasPayer, uint256 amount);
    event SponsorSet(address indexed newSponsor);
    
    constructor(address _trustedSponsor) {
        trustedSponsor = _trustedSponsor;
        // Initialize some premium tokens for demonstration
        premiumBalances[address(this)] = 10000 * 1e18;
    }
    
    /**
     * @dev Set trusted sponsor (only owner in real implementation)
     */
    function setTrustedSponsor(address _sponsor) external {
        trustedSponsor = _sponsor;
        emit SponsorSet(_sponsor);
    }
    
    /**
     * @dev VULNERABLE FUNCTION: Premium service that checks gas payer
     * This is the target of the exploit - it trusts txGasPayer() without validating msg.sender
     */
    function premiumAction() external returns (bool) {
        // VULNERABILITY: Only checks gas payer, ignores actual caller
        address gasPayer = EXTENSION.txGasPayer();
        require(gasPayer == trustedSponsor, "Not sponsored by trusted sponsor");
        
        // Grant premium access - transfer tokens to caller
        uint256 rewardAmount = 1000 * 1e18;
        require(premiumBalances[address(this)] >= rewardAmount, "Insufficient premium balance");
        
        premiumBalances[address(this)] -= rewardAmount;
        premiumBalances[msg.sender] += rewardAmount;
        
        emit PremiumAccess(msg.sender, gasPayer, rewardAmount);
        return true;
    }
    
    /**
     * @dev SECURE VERSION: Proper access control that validates caller relationship
     */
    function securePremiumAction() external returns (bool) {
        address gasPayer = EXTENSION.txGasPayer();
        
        // SECURE: Validate that caller has relationship to gas payer
        require(gasPayer == trustedSponsor, "Not sponsored by trusted sponsor");
        require(msg.sender == trustedSponsor || isAuthorizedUser(msg.sender), "Unauthorized caller");
        
        uint256 rewardAmount = 1000 * 1e18;
        require(premiumBalances[address(this)] >= rewardAmount, "Insufficient premium balance");
        
        premiumBalances[address(this)] -= rewardAmount;
        premiumBalances[msg.sender] += rewardAmount;
        
        emit PremiumAccess(msg.sender, gasPayer, rewardAmount);
        return true;
    }
    
    /**
     * @dev Check if user is authorized (placeholder for real authorization logic)
     */
    function isAuthorizedUser(address user) public pure returns (bool) {
        // In real implementation, this would check user authorization
        // For PoC, we'll return false to demonstrate the vulnerability
        return false;
    }
    
    /**
     * @dev Get premium balance for an address
     */
    function getPremiumBalance(address account) external view returns (uint256) {
        return premiumBalances[account];
    }
    
    /**
     * @dev Emergency function to check current gas payer (for debugging)
     */
    function getCurrentGasPayer() external view returns (address) {
        return EXTENSION.txGasPayer();
    }
}